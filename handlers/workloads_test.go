package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/business"
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
	namespace := "default"
	workload := "productpage-v1"
	context := "cluster03"
	prom, namespaceInfo := InitContextClientsForMetrics(defaultNoAuthPromClientSupplier, namespace, context)
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
