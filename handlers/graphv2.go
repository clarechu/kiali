package handlers

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
)

type GraphController struct {
	Config    *rest.Config
	ClientSet kubernetes.Interface
}

func NewGraphController(config *rest.Config, client kubernetes.Interface) *GraphController {
	return &GraphController{
		Config:    config,
		ClientSet: client,
	}
}

//GetNamespaces
// graph/namespace/demo,poc-demo/
func (g *GraphController) GetNamespaces(w http.ResponseWriter, r *http.Request) {
	url := r.RequestURI
	fmt.Println(url)
}

//GetNode
// graph/namespace/demo,poc-demo/app/xxxx/version/v1
// graph/namespace/demo,poc-demo/service/xxxx/version/
func (g *GraphController) GetNode(w http.ResponseWriter, r *http.Request) {

}
