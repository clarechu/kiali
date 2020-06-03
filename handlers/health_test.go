package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/url"
	"testing"
	"time"
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
	//context := "cluster03"
	business, err := GetBusinessNoAuth(nil, "")
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
	//context := "cluster01"
	business, err := GetBusinessNoAuth(nil, "")
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
	// Get business layer
	//context := "cluster01"
	business, err := GetBusinessNoAuth(GetRestConfig(), "http://10.10.13.39:30580")
	assert.Equal(t, nil, err)
	p := NamespaceHealthParams{
		Type: "service",
		BaseHealthParams: BaseHealthParams{
			Namespace:    "bookinfo",
			RateInterval: "60s",
			QueryTime:    time.Now(),
		},
	}
	ok, _ := p.Extract()
	assert.Equal(t, true, ok)
	// Adjust rate interval
	rateInterval, err := AdjustRateIntervalNoAuth(business, p.Namespace, p.RateInterval, p.QueryTime)
	assert.Equal(t, nil, err)

	switch p.Type {
	case "app":
		health, err := business.Health.GetNamespaceAppHealth(p.Namespace, rateInterval, p.QueryTime)
		assert.Equal(t, nil, err)
		b, _ := json.MarshalIndent(health, "", "")
		fmt.Println(string(b))
	case "service":
		health, err := business.Health.GetNamespaceServiceHealth(p.Namespace, rateInterval, p.QueryTime)
		assert.Equal(t, nil, err)
		b, _ := json.MarshalIndent(health, "", "")
		fmt.Println(string(b))
	case "workload":
		health, err := business.Health.GetNamespaceWorkloadHealth(p.Namespace, rateInterval, p.QueryTime)
		assert.Equal(t, nil, err)
		b, _ := json.MarshalIndent(health, "", "")
		fmt.Println(string(b))
	}

}
