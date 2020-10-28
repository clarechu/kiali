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
	"github.com/kiali/kiali/handlers"
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
	http.HandleFunc("/", GraphNode)
	return http.ListenAndServe(":8000", nil)
}

func GraphNode(w http.ResponseWriter, r *http.Request) {
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
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster01", "http://10.10.13.30:9090", clusters),
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster02", "http://10.10.13.34:9090", clusters),
		graph.NewSimpleOption("60s", "versionedApp", "app", "poc,poc-demo", "cluster03", "http://10.10.13.59:9090", clusters),
	}
	wg := sync.WaitGroup{}
	wg.Add(len(options))
	clusterCha := make(chan map[string]*PlayLoad, 10)
	for _, option := range options {
		go func(option graph.Option) {
			log.Infof("cluster start ")
			GraphNodeCluster(optionSpan, option, clusterCha)
			log.Infof("cluster :%v done ... ", option)
			wg.Done()
		}(option)
	}
	wg.Wait()
	p := make(map[string]interface{}, 0)
	for {
		select {
		case ss := <-clusterCha:
			for k, v := range ss {
				p[k] = v.Load
				_, ok := p["passthrough"]
				if !ok {
					p["passthrough"] = v.Edges
				} else {
					edges := p["passthrough"].([]*cytoscape.EdgeWrapper)
					edges = append(edges, v.Edges...)
					p["passthrough"] = edges
				}
			}

			if len(p) == len(options)+1 {
				b, _ := json.MarshalIndent(p, "", "")
				w.Write(b)
				return
			}
		}
	}
}

type PlayLoad struct {
	Load  interface{}              `json:"load"`
	Edges []*cytoscape.EdgeWrapper `json:"edges"`
}

//GraphNodeCluster 单个集群的流量视图
func GraphNodeCluster(optionSpan opentracing.Span, option graph.Option, cha chan map[string]*PlayLoad) {
	optionSpan.SetTag("namespaces", "poc-demo,poc")
	optionSpan.LogKV("new graph options", fmt.Sprintf("prometheus address:%s", "http://10.10.13.30:9090"))

	payload, edges, err := GetGraph(optionSpan, option)
	p := make(map[string]*PlayLoad, 0)
	if err != nil {
		p[option.Context] = nil
		cha <- p
		return
	}
	p[option.Context] = &PlayLoad{
		Load:  payload,
		Edges: edges,
	}
	cha <- p
}

//GetGraph 调用GraphNamespaces 的接口
func GetGraph(optionSpan opentracing.Span, option graph.Option) (result interface{}, edges []*cytoscape.EdgeWrapper, err error) {
	optionSpan.LogKV("new graph options", fmt.Sprintf("prometheus address:%s", option.Prometheus))
	o, err := option.NewGraphOptions(GetRestConfig(), option.Prometheus)
	optionSpan.LogKV("new graph options", fmt.Sprintf("end error:%v", err))
	optionSpan.Finish()

	businessSpan := opentracing.StartSpan("business", opentracing.FollowsFrom(optionSpan.Context()))
	if err != nil {
		return nil, nil, err
	}
	business, err := handlers.GetBusinessNoAuth(GetRestConfig(), option.Prometheus, optionSpan.Context())
	businessSpan.Finish()
	graphNamespacesSpan := opentracing.StartSpan("get graph", opentracing.FollowsFrom(businessSpan.Context()))
	graphNamespacesSpan.LogKV("func GraphNamespaces start", "")
	if err != nil {
		return
	}
	_, payload, edges, err := api.GraphNamespaces(business, o, graphNamespacesSpan)
	graphNamespacesSpan.Finish()
	return payload, edges, err
}

func GetRestConfig() (restConfig *rest.Config) {
	path := "/Users/clare/.kube/config"
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
