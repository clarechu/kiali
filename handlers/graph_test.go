package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/api"
	"github.com/kiali/kiali/kubernetes"
	"io/ioutil"
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"
	"os"
	"testing"
)

func GetRestConfig() (restConfig *rest.Config) {
	path := "/Users/clare/config"
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

func TestGraphNode(t *testing.T) {
	config.Set(config.NewConfig())
	option := graph.Option{
		Duration:           "60s",
		GraphType:          "versionedApp",
		InjectServiceNodes: "true",
		GroupBy:            "app",
		Appenders:          "deadNode,sidecarsCheck,serviceEntry,istio,unusedNode,securityPolicy",
		Namespaces:         "foo,kong,istio-system",
		Context:            "cluster01",
	}

	o := option.NewGraphOptions(GetRestConfig(), "http://10.10.13.30:21210")
	business, err := GetBusinessNoAuth(GetRestConfig(), "http://10.10.13.30:21210")
	graph.CheckError(err)
	code, payload := api.GraphNamespaces(business, o)
	fmt.Print(code)
	b, _ := json.MarshalIndent(payload, "", "")
	fmt.Println(string(b))
}

func TestGraphNode1(t *testing.T) {
	config.Set(config.NewConfig())
	r := &http.Request{
		Method:     http.MethodGet,
		Host:       "localhost:8577",
		RequestURI: "/graph/list?duration=60s&graphType=versionedApp&injectServiceNodes=true&groupBy=app&appenders=deadNode,sidecarsCheck,serviceEntry,istio,unusedNode,securityPolicy&namespaces=default&context=cluster03",
		URL: &url.URL{
			Path:     "/graph/list",
			RawQuery: "duration=60s&graphType=versionedApp&injectServiceNodes=true&groupBy=app&appenders=deadNode,sidecarsCheck,serviceEntry,istio,unusedNode,securityPolicy&namespaces=default&context=cluster03",
		},
	}

	o := graph.NewOptions(r)
	business, err := GetBusinessNoAuth(nil, "")
	graph.CheckError(err)
	code, payload := api.GraphNamespaces(business, o)
	fmt.Print(code)
	b, _ := json.MarshalIndent(payload, "", "")
	fmt.Println(string(b))
}
