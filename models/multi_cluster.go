package models

type MultiClusterEdge struct {
	Id            string `json:"id"`
	SourceId      string `json:"source_id"`
	DestinationId string `json:"destination_id"`
	Protocol      string `json:"protocol"`
	// 目标集群
	DestinationContext string `json:"destination_context"`
	// 源集群
	SourceContext string `json:"source_context"`
	Metadata map[MetadataKey]interface{} `json:"metadata"`
}

// MetadataKey is a mnemonic type name for string
type MetadataKey string