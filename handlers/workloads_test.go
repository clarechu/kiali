package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/prometheus"
	"github.com/stretchr/testify/assert"
	"testing"
)

// http://10.10.13.38/kiali/api/namespaces/default/workloads/productpage-v1/dashboard?
//reporter=destination&
//direction=inbound&
//avg=true&
//quantiles[]=0.5&quantiles[]=0.95&quantiles[]=0.99
//&duration=10800&
//step=216&
//rateInterval=216s
func TestWorkloadDashboard(t *testing.T) {
	config.Set(config.NewConfig())
	namespace := "default"
	workload := "productpage-v1"
	context := "cluster03"
	prom, namespaceInfo := InitContextClientsForMetrics(DefaultNoAuthPromClientSupplier, namespace, context)
	assert.NotEqual(t, prom, nil)
	eim := &ExtractIstioMetrics{
		Reporter:     "destination",
		Direction:    "inbound",
		Avg:          "true",
		Quantiles:    []string{"0.5", "0.95", "0.99"},
		Duration:     "10800",
		Step:         "216",
		RateInterval: "216s",
	}
	params := prometheus.IstioMetricsQuery{Namespace: namespace, Workload: workload}
	err := eim.ExtractIstioMetricsQueryParams(&params, namespaceInfo)
	assert.Equal(t, nil, err)

	svc := business.NewDashboardsService(prom)
	dashboard, err := svc.GetIstioDashboard(params)
	b, _ := json.MarshalIndent(dashboard, "", "")
	fmt.Println(string(b))
}

//api/namespaces/default/workloads/reviews-v1/metrics?
//queryTime=1590657111&
//duration=300&
//step=30&
//rateInterval=30s&
//filters[]=request_count&
//filters[]=request_duration&
//filters[]=request_duration_millis&
//filters[]=request_error_count&
//quantiles[]=0.5&
//quantiles[]=0.95&
//quantiles[]=0.99&
//byLabels[]=destination_service_name&
//direction=inbound
//&reporter=destination&
//requestProtocol=http
func TestWorkloadMetrics(t *testing.T) {
	namespace := "default"
	workload := "productpage-v1"
	context := "cluster03"
	prom, namespaceInfo := InitClientsForMetrics(DefaultNoAuthPromClientSupplier, namespace, context)
	if prom == nil {
		// any returned value nil means error & response already written
		return
	}
	eim := &ExtractIstioMetrics{
		Reporter:        "destination",
		RequestProtocol: "http",
		ByLabels:        []string{"destination_service_name"},
		Direction:       "inbound",
		Avg:             "true",
		Quantiles:       []string{"0.5", "0.95", "0.99"},
		Duration:        "300",
		QueryTime:       "1590657111",
		Step:            "30",
		RateInterval:    "30s",
		Filters: []string{
			"request_count",
			"request_duration",
			"request_duration_millis",
			"request_error_count",
		},
	}
	params := prometheus.IstioMetricsQuery{Namespace: namespace, Workload: workload}
	err := eim.ExtractIstioMetricsQueryParams(&params, namespaceInfo)
	assert.Equal(t, nil, err)
	metrics := prom.GetMetrics(&params)
	b, _ := json.MarshalIndent(metrics, "", "")
	fmt.Println(string(b))
}
