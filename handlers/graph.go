package handlers

// Graph.go provides handlers for graph request endpoints.   The handlers access vendor-specific
// telemetry (default istio) and return vendor-specific configuration (default cytoscape). The
// configuration format depends on the vendor but is typically JSON and provides what is necessary
// to allow the vendor's tool to render the graph.
//
// The algorithm is two-phased:
//   Phase One: Generate a TrafficMap using the requested TelemetryVendor. This typically queries
//              Prometheus, Istio and Kubernetes.
//
//   Phase Two: Provide the TrafficMap to the requested ConfigVendor which returns the vendor-specific
//              configuration returned to the caller.
//
// The current Handlers:
//   GraphNamespaces: Generate a graph for one or more requested namespaces.
//   GraphNode:       Generate a graph for a specific node, detailing the immediate incoming and outgoing traffic.
//
// The handlers accept the following query parameters (see notes below)
//   appenders:       Comma-separated list of TelemetryVendor-specific appenders to run. (default: all)
//   configVendor:    default: cytoscape
//   duration:        time.Duration indicating desired query range duration, (default: 10m)
//   graphType:       Determines how to present the telemetry data. app | service | versionedApp | workload (default: workload)
//   groupBy:         If supported by vendor, visually group by a specified node attribute (default: version)
//   namespaces:      Comma-separated list of namespace names to use in the graph. Will override namespace path param
//   queryTime:       Unix time (seconds) for query such that range is queryTime-duration..queryTime (default now)
//   TelemetryVendor: default: istio
//
//  Note: some handlers may ignore some query parameters.
//  Note: vendors may support additional, vendor-specific query parameters.
//

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/graph/config/cytoscape"
	"github.com/kiali/kiali/kubernetes"
	"github.com/opentracing/opentracing-go"
	"io/ioutil"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"runtime/debug"
	"sync"

	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/api"
	"github.com/kiali/kiali/log"
)

// GraphNamespaces is a REST http.HandlerFunc handling graph generation for 1 or more namespaces
// 查询流量图 这里这里
/*func GraphNamespaces(w http.ResponseWriter, r *http.Request) {
	defer handlePanic(w)

	o := graph.NewOptions(r)

	business, err := getBusiness(r)
	graph.CheckError(err)

	code, payload, err := api.GraphNamespaces(business, o, nil)
	respond(w, code, payload)
}

// GraphNode is a REST http.HandlerFunc handling node-detail graph config generation.
func GraphNode(w http.ResponseWriter, r *http.Request) {
	defer handlePanic(w)

	o := graph.NewOptions(r)

	business, err := getBusiness(r)
	graph.CheckError(err)

	code, payload := api.GraphNode(business, o)
	respond(w, code, payload)
}*/

func handlePanic(w http.ResponseWriter) {
	code := http.StatusInternalServerError
	if r := recover(); r != nil {
		var message string
		switch err := r.(type) {
		case string:
			message = err
		case error:
			message = err.Error()
		case func() string:
			message = err()
		case graph.Response:
			message = err.Message
			code = err.Code
		default:
			message = fmt.Sprintf("%v", r)
		}
		if code == http.StatusInternalServerError {
			stack := debug.Stack()
			log.Errorf("%s: %s", message, stack)
			RespondWithDetailedError(w, code, message, string(stack))
			return
		}
		RespondWithError(w, code, message)
	}
}

func respond(w http.ResponseWriter, code int, payload interface{}) {
	if code == http.StatusOK {
		RespondWithJSONIndent(w, code, payload)
		return
	}
	RespondWithError(w, code, payload.(string))
}

//var HOME_DIR = "/root/.kube/"

var HOME_DIR = "/Users/clare/.kube/"

type GraphNamespacesResponse struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    GraphName `json:"data"`
}

type GraphName struct {
	Cluster     interface{} `json:"cluster"`
	Passthrough interface{} `json:"passthrough"`
}

//GraphNamespace
func GraphNamespaces(w http.ResponseWriter, r *http.Request) {
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
		graph.NewSimpleOption("poc-demo", "cluster02", "http://10.10.13.30:9090",
			clusters, GetRestConfig("config")).SetDeadEdges(true),
		graph.NewSimpleOption("poc-demo", "cluster01", "http://10.10.13.34:9090",
			clusters, GetRestConfig("config_34")).SetDeadEdges(true),
		graph.NewSimpleOption("poc-demo", "cluster03", "http://10.10.13.59:9090",
			clusters, GetRestConfig("config_59")).SetDeadEdges(true),
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
	resp := &GraphNamespacesResponse{
		Code:    200,
		Message: "success",
		Data: GraphName{
			Cluster:     clusterCha,
			Passthrough: edges,
		},
	}
	b, _ := json.MarshalIndent(resp, "", "")
	w.Write(b)
}

// GraphNode 只展示当前
func GraphNode(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO()
	graphSpan, ctx := opentracing.StartSpanFromContext(ctx, fmt.Sprintf("HTTP GET /graph/context/{context}/node/{namespace}"))
	defer graphSpan.Finish()
	optionSpan := opentracing.StartSpan("node-options", opentracing.ChildOf(graphSpan.Context()))
	clusters := map[string]string{
		"cluster01": "10.10.13.34",
		"cluster02": "10.10.13.30",
		"cluster03": "10.10.13.59",
	}
	options := []graph.Option{
		graph.NewSimpleOption("",
			"cluster01", "http://10.10.13.30:9090", clusters, GetRestConfig("config")).
			//SetApp("greeter-server", "v1").
			SetService("greeter-client").
			SetDeadEdges(true).
			SetNamespace("poc-demo"),
		graph.NewSimpleOption("",
			"cluster02", "http://10.10.13.34:9090", clusters, GetRestConfig("config_34")).
			//SetApp("greeter-server", "v1").
			SetService("greeter-client").
			SetDeadEdges(true).
			SetNamespace("poc-demo"),
		graph.NewSimpleOption("",
			"cluster03", "http://10.10.13.59:9090", clusters, GetRestConfig("config_59")).
			//SetApp("greeter-server", "v1").
			SetService("greeter-client").
			SetDeadEdges(true).
			SetNamespace("poc-demo"),
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
			e, err := graphApi.NodeRegistryHandle(optionSpan, clusterCha)
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
	//passthroughcluster
	clusterCha["passthrough"] = edges
	b, _ := json.MarshalIndent(clusterCha, "", "")
	w.Write(b)
}

func GetRestConfig(name string) (restConfig *rest.Config) {
	//path := "/Users/clare/.kube/" + name
	path := HOME_DIR + name
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
