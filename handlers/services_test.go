package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/kiali/kiali/prometheus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestServicesMetrics(t *testing.T) {
	namespace := "default"
	service := "default"
	context := "cluster03"
	prom, namespaceInfo := InitClientsForMetrics(DefaultNoAuthPromClientSupplier, namespace, context)
	if prom == nil {
		// any returned value nil means error & response already written
		return
	}

	params := prometheus.IstioMetricsQuery{Namespace: namespace, Service: service}
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
	err := eim.ExtractIstioMetricsQueryParams(&params, namespaceInfo)
	assert.Equal(t, nil, err)
	metrics := prom.GetMetrics(&params)
	b, _ := json.MarshalIndent(metrics, "", "")
	fmt.Println(string(b))
}
