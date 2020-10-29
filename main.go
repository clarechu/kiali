package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/api"
	"github.com/kiali/kiali/graph/config/cytoscape"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/util"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
	"io/ioutil"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"sync"
)

func init() {
	// log everything to stderr so that it can be easily gathered by logs, separate log files are problematic with containers
	_ = flag.Set("logtostderr", "true")
	flag.Set("v", "5")

}

// Command line arguments
var (
	argConfigFile = flag.String("config", "", "Path to the YAML configuration file. If not specified, environment variables will be used for configuration.")
)

func main() {
	defer glog.Flush()
	util.Clock = util.RealClock{}

	// process command line
	flag.Parse()
	// load config file if specified, otherwise, rely on environment variables to configure us
	if *argConfigFile != "" {
		c, err := config.LoadFromFile(*argConfigFile)
		if err != nil {
			glog.Fatal(err)
		}
		config.Set(c)
	} else {
		log.Infof("No configuration file specified. Will rely on environment for configuration.")
		config.Set(config.NewConfig())
	}

	log.Tracef("Kiali Configuration:\n%+v", config.Get().Server.Address)
	log.Errorf("server start %v", NewServer())

}

func NewServer() error {
	cfg := jaegercfg.Configuration{
		ServiceName: "kiali",
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans:           true,
			LocalAgentHostPort: "10.10.13.30:26034", // 替换host
		},
	}
	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory
	tracer, _, err := cfg.NewTracer(
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	opentracing.SetGlobalTracer(tracer)
	if err != nil {
		log.Errorf("Could not initialize jaeger tracer: %s", err.Error())
		return err
	}
	http.HandleFunc("/", GraphNamespace)
	return http.ListenAndServe(":8000", nil)
}

func GraphNamespace(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO()
	graphSpan, ctx := opentracing.StartSpanFromContext(ctx, fmt.Sprintf("HTTP GET /graph/context/{context}/namespaces/{namespace}"))
	defer graphSpan.Finish()
	optionSpan := opentracing.StartSpan("options", opentracing.ChildOf(graphSpan.Context()))
	clusters := map[string]string{
		"cluster01": "10.10.13.34",
		"cluster02": "10.10.13.30",
		"cluster03": "10.10.13.59",
	}
	options := []graph.Option{
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster01", "http://10.10.13.30:9090", clusters, GetRestConfig("config")),
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster02", "http://10.10.13.34:9090", clusters, GetRestConfig("config_34")),
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster03", "http://10.10.13.59:9090", clusters, GetRestConfig("config_59")),
	}
	wg := sync.WaitGroup{}
	wg.Add(len(options))
	clusterCha := make(map[string]interface{}, 0)
	edges := make([]*cytoscape.EdgeWrapper, 0)
	for _, option := range options {
		go func(option graph.Option) {
			log.Infof("cluster start ")
			graphApi, err := api.NewGraphApi(option, optionSpan)
			if err != nil {
				wg.Done()
				return
			}
			// handle
			e, err := graphApi.RegistryHandle(optionSpan, clusterCha)
			if err != nil {
				wg.Done()
				return
			}
			edges = append(edges, e...)
			log.Info("cluster :%v done ... ")
			wg.Done()
		}(option)
	}
	wg.Wait()
	optionSpan.Finish()
	clusterCha["passthrough"] = edges
	b, _ := json.MarshalIndent(clusterCha, "", "")
	w.Write(b)
}

func GetRestConfig(name string) (restConfig *rest.Config) {
	path := "/Users/clare/.kube/" + name
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	fd, err := ioutil.ReadAll(file)
	cf, err := kubernetes.LoadFromFile(fd)
	restConfig = &rest.Config{
		Host: cf.Clusters[cf.Contexts[cf.CurrentContext].Cluster].Server,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   cf.Clusters[cf.Contexts[cf.CurrentContext].Cluster].CertificateAuthorityData,
			CertData: cf.AuthInfos[cf.Contexts[cf.CurrentContext].AuthInfo].ClientCertificateData,
			KeyData:  cf.AuthInfos[cf.Contexts[cf.CurrentContext].AuthInfo].ClientKeyData,
		},
		//Timeout: time.Second * 5,
	}
	return restConfig
}
