package handlers

import (
	"context"
	"fmt"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/api"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/util"
	"github.com/opentracing/opentracing-go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
)

type GraphController struct {
	Context       string
	PrometheusURL string
	Config        *rest.Config
	ClientSet     kubernetes.Interface
}

func NewGraphController(config *rest.Config, client kubernetes.Interface) *GraphController {
	return &GraphController{
		Config:        config,
		ClientSet:     client,
		PrometheusURL: "http://10.10.13.30:9090",
		Context:       "cluster",
	}
}

type Graph struct {
	Namespace string `json:"namespace" default:"default"`
	// 集群名称
	DeadEdges     bool   `json:"deadEdges" default:"false"`
	PrometheusUrl string `json:"prometheusUrl"`
	Service       string `json:"service"`
	App           string `json:"app"`
	Version       string `json:"version"`
	PassThrough   bool   `json:"passThrough" default:"true"`
	Duration      string `json:"duration" default:"60s"`
}

// @ID GetNamespaces
// @Summary graph-namespace
// @Description 通过namespace来查询流量视图
// @Accept  json
// @Param namespace path string true "命名空间"
// @Param duration path string true "时长"
// @Param deadEdges path boolean false "是否去掉没有流量的线"
// @Param passThrough path boolean false "是否需要加多集群的线"
// @Success 200 {object} GraphNamespacesResponse
// @Failure 500 {object} responseError
// @Router /graph/namespace/{namespace}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough} [get]
func (g *GraphController) GetNamespacesController(w http.ResponseWriter, r *http.Request) {

	url := r.RequestURI
	url = url[7:]
	graphs := &Graph{}
	err := util.Parse(url, graphs)
	if err != nil {
		RespondWithError(w, 500, err.Error())
		return
	}
	graphName, err := g.GetNamespaces(graphs)
	if err != nil {
		RespondWithError(w, 500, err.Error())
		return
	}
	RespondWithJSON(w, 200, graphName)
}

func (g *GraphController) GetNamespaces(graphs *Graph) (graphName GraphName, err error) {
	ctx := context.TODO()
	graphSpan, ctx := opentracing.StartSpanFromContext(ctx, fmt.Sprintf("GetNamespaces"))
	defer graphSpan.Finish()
	optionSpan := opentracing.StartSpan("namespace-options", opentracing.ChildOf(graphSpan.Context()))
	clusters := map[string]string{
		"cluster01": "10.10.13.34",
		"cluster02": "10.10.13.30",
		"cluster03": "10.10.13.59",
	}
	option := graph.NewSimpleOption(graphs.Namespace, g.Context, g.PrometheusURL,
		clusters, g.Config).SetDeadEdges(graphs.DeadEdges).SetPassThrough(graphs.PassThrough).SetDuration(graphs.Duration)
	clusterCha := make(map[string]interface{}, 0)
	log.Infof("cluster start ")
	graphApi, err := api.NewGraphApi(option, optionSpan)
	if err != nil {
		return
	}
	// handle
	edges, err := graphApi.RegistryHandle(optionSpan, clusterCha)
	if err != nil {
		return
	}
	log.Info("cluster done ... ")
	graphName = GraphName{
		Cluster:     clusterCha[g.Context],
		Passthrough: edges,
	}
	return
}

func (g *GraphController) GetNode() {

}

//GetNodeToService
// graph/namespace/demo,poc-demo/app/xxxx/version/v1
// graph/namespace/demo,poc-demo/service/xxxx/version/
// @ID GetNode
// @Summary graph-Node
// @Description 通过node来查询流量视图
// @Accept  json
// @Param namespace path string true "命名空间"
// @Param duration path string true "时长"
// @Param service path string true "service 名称"
// @Param deadEdges path boolean false "是否去掉没有流量的线"
// @Param passThrough path boolean false "是否需要加多集群的线"
// @Success 200 {object} GraphNamespacesResponse
// @Failure 500 {object} responseError
// @Router /graph/namespace/{namespace}/service/{service}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough} [get]
func (g *GraphController) GetNodeController(w http.ResponseWriter, r *http.Request) {
	url := r.RequestURI
	url = url[7:]
	graphs := &Graph{}
	err := util.Parse(url, graphs)
	if err != nil {
		RespondWithError(w, 500, err.Error())
		return
	}
	RespondWithJSON(w, 200, 1)
}
