package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/url"
	"testing"
)

func TestServiceHealth(t *testing.T) {
	r := &http.Request{
		Method:     http.MethodGet,
		Host:       "localhost:8577",
		RequestURI: "namespaces/default/health?type=service&rateInterval=60s&context=cluster03",
		URL: &url.URL{
			Path:     "/namespaces/default/health",
			RawQuery: "type=service&rateInterval=60s&context=cluster03",
		},
	}
	context := "cluster03"
	business, err := GetBusinessNoAuth(context)
	assert.Equal(t, nil, err)

	p := serviceHealthParams{}
	p.extract(r)
	p.Namespace = "default"
	p.Service = "productpage"
	rateInterval, err := AdjustRateIntervalNoAuth(business, p.Namespace, p.RateInterval, p.QueryTime)
	assert.Equal(t, nil, err)
	health, err := business.Health.GetServiceHealth(p.Namespace, p.Service, rateInterval, p.QueryTime)
	b, _ := json.MarshalIndent(health, "", "")
	fmt.Println(string(b))
}

func TestAppHealth(t *testing.T) {
	context := "cluster01"
	business, err := GetBusinessNoAuth(context)
	assert.Equal(t, nil, err)
	p := AppHealthParams{}
	p.Extract("kubernetes", "60s", "bookinfo")
	rateInterval, err := AdjustRateIntervalNoAuth(business, p.Namespace, p.RateInterval, p.QueryTime)
	assert.Equal(t, nil, err)
	health, err := business.Health.GetAppHealth(p.Namespace, p.App, rateInterval, p.QueryTime)
	b, _ := json.MarshalIndent(health, "", "")
	fmt.Println(string(b))
}

func TestWorkloadHealth(t *testing.T) {

}

func TestNamespaceHealth(t *testing.T) {
/*	// Get business layer
	context := "cluster01"
	business, err := GetBusinessNoAuth(context)
	assert.Equal(t, nil, err)
	p := namespaceHealthParams{}
	ok, err := p.extract(r)
	// Adjust rate interval
	rateInterval, err := adjustRateInterval(business, p.Namespace, p.RateInterval, p.QueryTime)
	assert.Equal(t, nil, err)

	switch p.Type {
	case "app":
		health, err := business.Health.GetNamespaceAppHealth(p.Namespace, rateInterval, p.QueryTime)

	case "service":
		health, err := business.Health.GetNamespaceServiceHealth(p.Namespace, rateInterval, p.QueryTime)

	case "workload":
		health, err := business.Health.GetNamespaceWorkloadHealth(p.Namespace, rateInterval, p.QueryTime)

	}*/
}
