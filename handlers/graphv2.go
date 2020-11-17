package handlers

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
)

type GraphController struct {
	Context   string
	Config    *rest.Config
	ClientSet kubernetes.Interface
}

func NewGraphController(config *rest.Config, client kubernetes.Interface) *GraphController {
	return &GraphController{
		Config:    config,
		ClientSet: client,
	}
}

type Graph struct {
	Namespaces []string `json:"namespaces" default:"default"`
	// 集群名称
	DeadEdges     bool   `json:"deadEdges" default:"false"`
	PrometheusUrl string `json:"prometheusUrl"`
	Service       string `json:"service"`
	App           string `json:"app"`
	Version       string `json:"version"`
	PassThrough   bool   `json:"passThrough" default:"true"`
	Duration      string `json:"duration" default:"60s"`
}

// ShowAccount godoc
// @Tag.name graph
// @ID get-string-by-int
// @Summary graph-namespace
// @Description 通过namespace来查询流量视图
// @Accept  json
// @Param namespace path string true "命名空间"
// @Param duration path string true "时长"
// @Param deadEdges path boolean false "是否去掉没有流量的线"
// @Param passThrough path boolean false "是否需要加多集群的线"
// @Success 200 {object} GraphNamespacesResponse
// @Router /graph/namespace/{namespace}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough} [get]
func (g *GraphController) GetNamespaces(w http.ResponseWriter, r *http.Request) {
	url := r.RequestURI
	fmt.Println(url)
}

//GetNode
// graph/namespace/demo,poc-demo/app/xxxx/version/v1
// graph/namespace/demo,poc-demo/service/xxxx/version/
func (g *GraphController) GetNode(w http.ResponseWriter, r *http.Request) {

}
