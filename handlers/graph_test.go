package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/api"
	"net/http"
	"net/url"
	"testing"
)

func TestGraphNode(t *testing.T) {
	r := &http.Request{
		Method:     http.MethodGet,
		Host:       "localhost:8577",
		RequestURI: "/graph/list?duration=60s&graphType=versionedApp&injectServiceNodes=true&groupBy=app&appenders=deadNode,sidecarsCheck,serviceEntry,istio,unusedNode,responseTime&namespaces=bookinfo&context=cluster01",
		URL: &url.URL{
			Path:     "/graph/list",
			RawQuery: "duration=60s&graphType=versionedApp&injectServiceNodes=true&groupBy=app&appenders=deadNode,sidecarsCheck,serviceEntry,istio,unusedNode,responseTime&namespaces=bookinfo&context=cluster01",
		},
	}
	o := graph.NewOptions(r)

	business, err := GetBusinessNoAuth(o.Context)
	graph.CheckError(err)
	code, payload := api.GraphNamespaces(business, o)
	fmt.Print(code)
	b, _ := json.MarshalIndent(payload, "", "")
	fmt.Println(string(b))
}
