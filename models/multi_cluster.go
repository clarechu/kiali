package models

import "k8s.io/client-go/rest"

type MultiClusterEdge struct {
	Id            string            `json:"id"`
	SourceId      string            `json:"source_id"`
	DestinationId string            `json:"destination_id"`
	Protocol      string            `json:"protocol"`
	Rate          map[string]string `json:"rate"`
	Host          string            `json:"host"`
	// 目标集群
	DestinationContext string `json:"destination_context"`
	// 源集群
	SourceContext string                      `json:"source_context"`
	Metadata      map[MetadataKey]interface{} `json:"metadata"`
	Code          string
}

// MetadataKey is a mnemonic type name for string
type MetadataKey string

type Cluster struct {
	Config             *rest.Config `json:"config"`
	Name               string       `json:"name"`
	PassThroughCluster []string     `json:"passThroughCluster"`
	PrometheusUrl      string       `json:"prometheusUrl"`
}
