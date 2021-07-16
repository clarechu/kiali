package models

type ReplicaseList struct {
	// Namespace where the workloads live in
	// required: true
	// example: bookinfo
	Namespace Namespace `json:"namespace"`

	// Workloads for a given namespace
	// required: true
	Workloads []ReplicaseListItem `json:"replicase"`
}

type ReplicaseListItem struct {

	// Name of the workload
	// required: true
	// example: reviews-v1
	Name string `json:"name"`

	// Type of the workload
	// required: true
	// example: deployment
	Type string `json:"type"`

	// Creation timestamp (in RFC3339 format)
	// required: true
	// example: 2018-07-31T12:24:17Z
	CreatedAt string `json:"createdAt"`

	// Kubernetes ResourceVersion
	// required: true
	// example: 192892127
	ResourceVersion string `json:"resourceVersion"`

	// Define if Pods related to this Workload has an IstioSidecar deployed
	// required: true
	// example: true
	IstioSidecar bool `json:"istioSidecar"`

	// Additional item sample, such as type of api being served (graphql, grpc, rest)
	// example: rest
	// required: false
	AdditionalDetailSample *AdditionalItem `json:"additionalDetailSample"`

	// Workload labels
	Labels map[string]string `json:"labels"`

	// Define if Pods related to this Workload has the label App
	// required: true
	// example: true
	AppLabel bool `json:"appLabel"`

	// Define if Pods related to this Workload has the label Version
	// required: true
	// example: true
	VersionLabel bool `json:"versionLabel"`

	// Number of current workload pods
	// required: true
	// example: 1
	PodCount int `json:"podCount"`
}
