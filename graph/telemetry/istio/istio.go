// Package istio provides the Istio implementation of graph/TelemetryProvider.
package istio

// Istio.go is responsible for generating TrafficMaps using Istio telemetry.  It implements the
// TelemetryVendor interface.
//
// The algorithm is two-pass:
//   First Pass: Query Prometheus (istio-requests-total metric) to retrieve the source-destination
//               dependencies. Build a traffic map to provide a full representation of nodes and edges.
//
//   Second Pass: Apply any requested appenders to alter or append to the graph.
//
//
// Supports one vendor-specific query parameters:
//   responseTimeQuantile: Must be a valid quantile (default: 0.95)
//
import (
	"context"
	"fmt"
	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/models"
	"github.com/opentracing/opentracing-go"
	"strings"
	"time"

	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/graph/telemetry"
	"github.com/kiali/kiali/graph/telemetry/istio/appender"
	"github.com/kiali/kiali/graph/telemetry/istio/util"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/prometheus/internalmetrics"
	"github.com/kiali/kiali/status"
)

// version-specific telemetry field names.  Because the istio version can change outside of the kiali pod,
// these values may change and are therefore re-set on every graph request.
var appLabel = "app"
var verLabel = "version"

func setLabels() {
	if status.AreCanonicalMetricsAvailable() {
		appLabel = "canonical_service"
		verLabel = "canonical_revision"
	}
}

// BuildNamespacesTrafficMap is required by the graph/TelemtryVendor interface
func BuildNamespacesTrafficMap(o graph.TelemetryOptions, client *prometheus.Client, globalInfo *graph.AppenderGlobalInfo, span opentracing.Span) graph.TrafficMap {
	log.Tracef("Build [%s] graph for [%v] namespaces [%s]", o.GraphType, len(o.Namespaces), o.Namespaces)

	appenders := appender.ParseAppenders(o)
	span.LogKV("parse appenders")
	// init TrafficMap
	trafficMap := graph.NewTrafficMap()

	for _, namespace := range o.Namespaces {
		log.Tracef("Build traffic map for namespace [%s]", namespace)
		//生成一个 namespaceTrafficMap
		span.LogKV("buildNamespaceTrafficMap start", "")
		namespaceTrafficMap := buildNamespaceTrafficMap(namespace.Name, o, client)
		span.LogKV("buildNamespaceTrafficMap end", "")
		namespaceInfo := graph.NewAppenderNamespaceInfo(namespace.Name)
		span.LogKV("NewAppenderNamespaceInfo end", "")

		for _, a := range appenders {
			appendersSpan := opentracing.StartSpan(a.Name(), opentracing.ChildOf(span.Context()))
			appenderTimer := internalmetrics.GetGraphAppenderTimePrometheusTimer(a.Name())
			appendersSpan.LogKV("appenders start", a.Name())
			err := a.AppendGraph(namespaceTrafficMap, globalInfo, namespaceInfo)
			appendersSpan.LogKV("appenders end", a.Name())
			if err != nil {
				break
			}
			appendersSpan.Finish()
			appenderTimer.ObserveDuration()
		}
		span.LogKV("appenders end ...")
		// 将 namespaceTrafficMap merge ---->  trafficMap 中
		telemetry.MergeTrafficMaps(trafficMap, namespace.Name, namespaceTrafficMap)
		span.LogKV("MergeTrafficMaps...")
	}

	// The appenders can add/remove/alter nodes. After the manipulations are complete
	// we can make some final adjustments:
	// - mark the outsiders (i.e. nodes not in the requested namespaces)
	// - mark the insider traffic generators (i.e. inside the namespace and only outgoing edges)
	telemetry.MarkOutsideOrInaccessible(trafficMap, o)
	telemetry.MarkTrafficGenerators(trafficMap)

	if graph.GraphTypeService == o.GraphType {
		trafficMap = telemetry.ReduceToServiceGraph(trafficMap)
	}

	return trafficMap
}

//添加多集群的线
func NodeMultiClusterEdge(o graph.Options, globalInfo *graph.AppenderGlobalInfo, clusters map[string]string, context string) ([]models.MultiClusterEdge, error) {
	edges := make([]models.MultiClusterEdge, 0)
	istioCfg, err := globalInfo.Business.IstioConfig.GetIstioConfigList(business.IstioConfigCriteria{
		IncludeServiceEntries: true,
		Namespace:             o.Namespace,
	})
	if err != nil {
		return nil, err
	}
	istioCountMetric := "istio_request_bytes_count"
	tcpGroupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,response_flags,response_code,request_protocol",
		appLabel, verLabel, appLabel, verLabel)
	query := fmt.Sprintf(`sum(rate(%s{destination_service=~".*global",source_workload_namespace="%s",response_code="200"} [%vs])) by (%s)`,
		istioCountMetric,
		o.Namespace,
		int(o.TelemetryOptions.Duration.Seconds()), // range duration for the query
		tcpGroupBy)
	//查询有几个跨集群流量
	tcpInVector := promQuery(query, time.Unix(o.TelemetryOptions.QueryTime, 0), globalInfo.PromClient.API())
	for _, s := range tcpInVector {
		m := s.Metric
		lDestinationSvc, destinationSvcOk := m["destination_service"]
		lSourceWl, sourceWlOk := m["source_workload"]
		code, _ := m["response_code"]
		lSourceApp, sourceAppOk := m["source_app"]
		lDestinationApp, _ := m["destination_app"]
		lDestinationWlNs, _ := m["destination_service_namespace"]
		lSourceVs, sourceVsOk := m["source_version"]
		lDestinationWl, destinationWlOk := m["destination_workload"]
		lSourceWlNs, sourceWlNsOk := m["source_workload_namespace"]
		lDestinationWl, destinationWlNsOk := m["destination_workload"]
		lProtocol, protocolOk := m["request_protocol"]
		if !destinationSvcOk || !sourceWlNsOk || !destinationWlNsOk || !sourceAppOk || !destinationWlOk || !sourceWlOk || !sourceVsOk || !protocolOk {
			log.Warning("not found sdt svc")
			continue
		}

		//todo 只保留 有目的地的线
		service := o.NodeOptions.Service
		app := o.NodeOptions.App
		if app != "" {
			if string(lDestinationApp) != app && string(lSourceApp) != app {
				continue
			}
		} else if service != "" {
			if strings.Split(string(lDestinationSvc), ".")[0] != service {
				continue
			}
		} else {
			continue
		}
		for _, se := range istioCfg.ServiceEntries {
			// host --> svc.ns.global
			for _, host := range se.Spec.Hosts.([]interface{}) {
				if string(lDestinationSvc) == host.(string) {
					hostSplitted := strings.Split(host.(string), ".")
					if len(hostSplitted) < 3 || hostSplitted[2] != "global" {
						break
					}
					if host.(string) == string(lDestinationSvc) {
						sourceId, _ := graph.Id(string(lSourceWlNs), string(lSourceApp), string(lSourceWlNs),
							string(lSourceWl), string(lSourceApp), string(lSourceVs), graph.GraphTypeVersionedApp)
						destinationId, _ := graph.Id(string(lDestinationWlNs), hostSplitted[0], "",
							"", hostSplitted[1], "", graph.GraphTypeService)
						log.Debugf("sourceId :%v, destinationId :%v lDestinationWl: %v lProtocol:%v", sourceId, destinationId, lDestinationWl, lProtocol)
						ips := multiClusters(se.Spec.Endpoints, clusters)
						for _, desContext := range ips {
							if desContext == context {
								continue
							}
							rate := map[string]string{}
							if s.Value.String() != "0" {
								rate = map[string]string{
									string(lProtocol): s.Value.String(),
									fmt.Sprintf("%s%s", string(lProtocol), "PercentReq"): fmt.Sprintf("%v", 100/len(se.Spec.Endpoints)),
								}
							}
							edge := models.MultiClusterEdge{
								SourceId:           sourceId,
								DestinationId:      destinationId,
								Protocol:           string(lProtocol),
								SourceContext:      context,
								DestinationContext: desContext,
								Rate:               rate,
								Code:               string(code),
								Host:               string(lDestinationSvc),
							}
							edges = append(edges, edge)
						}
					}
				}
			}
		}
	}
	return edges, nil
}

// 添加多集群的线
//todo 这个地方还需要搞一下 查询所有的集群的数据 在来聚合
func AddMultiClusterEdge(o graph.TelemetryOptions, globalInfo *graph.AppenderGlobalInfo, clusters map[string]string, context string) ([]models.MultiClusterEdge, error) {
	edges := make([]models.MultiClusterEdge, 0)
	for namespace := range o.Namespaces {
		istioCfg, err := globalInfo.Business.IstioConfig.GetIstioConfigList(business.IstioConfigCriteria{
			IncludeServiceEntries: true,
			Namespace:             namespace,
		})
		if err != nil {
			return nil, err
		}
		//todo 获取跨集群的数据
		istioCountMetric := "istio_request_bytes_count"
		tcpGroupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,response_flags,response_code,request_protocol",
			appLabel, verLabel, appLabel, verLabel)
		query := fmt.Sprintf(`sum(rate(%s{destination_service=~".*global",source_workload_namespace="%s",response_code="200"} [%vs])) by (%s)`,
			istioCountMetric,
			namespace,
			int(o.Duration.Seconds()), // range duration for the query
			tcpGroupBy)
		//查询有几个跨集群流量
		tcpInVector := promQuery(query, time.Unix(o.QueryTime, 0), globalInfo.PromClient.API())
		for _, s := range tcpInVector {
			m := s.Metric
			lDestinationSvc, destinationSvcOk := m["destination_service"]
			lSourceWl, sourceWlOk := m["source_workload"]
			code, _ := m["response_code"]
			lSourceApp, sourceAppOk := m["source_app"]
			lDestinationWlNs, _ := m["destination_service_namespace"]
			lSourceVs, sourceVsOk := m["source_version"]
			//lDestinationWl, destinationWlOk := m["destination_workload"]
			_, destinationWlOk := m["destination_workload"]
			lSourceWlNs, sourceWlNsOk := m["source_workload_namespace"]
			_, destinationWlNsOk := m["destination_workload"]
			lProtocol, protocolOk := m["request_protocol"]
			if !destinationSvcOk || !sourceWlNsOk || !destinationWlNsOk || !sourceAppOk || !destinationWlOk || !sourceWlOk || !sourceVsOk || !protocolOk {
				log.Warning("not found sdt svc")
				continue
			}
			for _, se := range istioCfg.ServiceEntries {
				// host --> svc.ns.global
				for _, host := range se.Spec.Hosts.([]interface{}) {
					if string(lDestinationSvc) == host.(string) {
						hostSplitted := strings.Split(host.(string), ".")
						if len(hostSplitted) < 3 || hostSplitted[2] != "global" {
							break
						}
						if host.(string) == string(lDestinationSvc) {
							sourceId, _ := graph.Id(string(lSourceWlNs), string(lSourceApp), string(lSourceWlNs),
								string(lSourceWl), string(lSourceApp), string(lSourceVs), graph.GraphTypeVersionedApp)
							destinationId, _ := graph.Id(string(lDestinationWlNs), hostSplitted[0], "",
								"", hostSplitted[1], "", graph.GraphTypeService)
							//							log.Debugf("sourceId :%v, destinationId :%v lDestinationWl: %v lProtocol:%v", sourceId, destinationId, lDestinationWl, lProtocol)
							ips := multiClusters(se.Spec.Endpoints, clusters)
							for _, desContext := range ips {
								if desContext == context {
									continue
								}
								rate := map[string]string{}
								if s.Value.String() != "0" {
									rate = map[string]string{
										string(lProtocol): s.Value.String(),
										fmt.Sprintf("%s%s", string(lProtocol), "PercentReq"): fmt.Sprintf("%v", 100/len(se.Spec.Endpoints)),
									}
								}
								edge := models.MultiClusterEdge{
									SourceId:           sourceId,
									DestinationId:      destinationId,
									Protocol:           string(lProtocol),
									SourceContext:      context,
									DestinationContext: desContext,
									Rate:               rate,
									Code:               string(code),
									Host:               string(lDestinationSvc),
								}
								edges = append(edges, edge)
							}
						}
					}
				}
			}
		}
	}
	return edges, nil
}

//multiClusters 需要展示的线的集群 multiCluster
func multiClusters(ips []models.ServiceEntriesEndpoints, clusters map[string]string) []string {
	cs := make([]string, 0)
	for k, v := range clusters {
		for _, ip := range ips {
			if v == ip.Address {
				cs = append(cs, k)
			}
		}
	}
	return cs
}

// buildNamespaceTrafficMap returns a map of all namespace nodes (key=id).  All
// nodes either directly send and/or receive requests from a node in the namespace.
// 返回所有名称空间节点（key = id）的映射。所有节点都直接从名称空间中的节点发送和/或接收请求。
func buildNamespaceTrafficMap(namespace string, o graph.TelemetryOptions, client *prometheus.Client) graph.TrafficMap {
	// create map to aggregate traffic by protocol and response code
	trafficMap := graph.NewTrafficMap()

	requestsMetric := "istio_requests_total"
	duration := o.Namespaces[namespace].Duration

	// We query for source telemetry (generated by the source proxy) because it includes client-side failures. But
	// traffic between mesh services and Istio components is not reported by proxy, it is generated as destination
	// telemetry by the Istio components directly.  So, we alter the queries as needed.
	isIstioNamespace := o.Namespaces[namespace].IsIstio

	// query prometheus for request traffic in three queries:
	// 1) query for traffic originating from "unknown" (i.e. the internet). Unknown sources have no istio sidecar so
	//    it is destination telemetry. Here we use destination_workload_namespace because destination telemetry
	//    always provides the workload namespace, and because destination_service_namespace is provided from the source,
	//    and for a request originating on a different cluster, will be set to the namespace where the service-entry is
	//    defined, on the other cluster.
	groupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,request_protocol,response_code,grpc_response_status,response_flags", appLabel, verLabel, appLabel, verLabel)
	query := fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload="unknown",destination_workload_namespace="%s"} [%vs])) by (%s)`,
		requestsMetric,
		namespace,
		int(duration.Seconds()), // range duration for the query
		groupBy)
	unkVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
	populateTrafficMap(trafficMap, &unkVector, o)

	// 2) query for external traffic, originating from a workload outside of the namespace.  Exclude any "unknown" source telemetry (an unusual corner
	//	  case resulting from pod lifecycle changes).  Here use destination_service_workload to capture failed requests never reaching a dest workload.
	reporter := "source"
	sourceWorkloadNamespaceQuery := fmt.Sprintf(`source_workload_namespace!="%s"`, namespace)
	if isIstioNamespace {
		// also exclude any non-requested istio namespaces
		reporter = "destination"
		excludedIstioNamespaces := config.GetIstioNamespaces(o.Namespaces.GetIstioNamespaces())
		if len(excludedIstioNamespaces) > 0 {
			excludedIstioRegex := strings.Join(excludedIstioNamespaces, "|")
			sourceWorkloadNamespaceQuery = fmt.Sprintf(`source_workload_namespace!~"%s|%s"`, namespace, excludedIstioRegex)
		}
	}
	query = fmt.Sprintf(`sum(rate(%s{reporter="%s",%s,source_workload!="unknown",destination_service_namespace="%s"} [%vs])) by (%s)`,
		requestsMetric,
		reporter,
		sourceWorkloadNamespaceQuery,
		namespace,
		int(duration.Seconds()), // range duration for the query
		groupBy)
	extVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
	populateTrafficMap(trafficMap, &extVector, o)

	// 3) query for internal traffic, originating from a workload inside of the namespace
	// 注释掉 reporter="source",
	query = fmt.Sprintf(`sum(rate(%s{source_workload_namespace="%s"} [%vs])) by (%s)`,
		requestsMetric,
		namespace,
		int(duration.Seconds()), // range duration for the query
		groupBy)
	intVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
	populateTrafficMap(trafficMap, &intVector, o)
	// Query3 misses istio-to-istio traffic, which is only reported destination-side, we must perform an additional query
	if isIstioNamespace {
		// find traffic from the source istio namespace to any of the requested istio namespaces
		includedIstioRegex := strings.Join(o.Namespaces.GetIstioNamespaces(), "|")

		// 3a) supplemental query for istio-to-istio traffic
		query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload_namespace="%s",destination_service_namespace=~"%s"} [%vs])) by (%s)`,
			requestsMetric,
			namespace,
			includedIstioRegex,
			int(duration.Seconds()), // range duration for the query
			groupBy)
		intIstioVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMap(trafficMap, &intIstioVector, o)
	}
	//todo
	for k, v := range trafficMap {
		if !strings.Contains(k, "unknown") {
			if _, ok := v.Metadata["httpIn"]; !ok {
				for _, edges := range v.Edges {
					delete(edges.Metadata, "http")
				}
			}
		}
	}
	// Section for TCP services (note, there is no TCP Istio traffic)
	tcpMetric := "istio_tcp_sent_bytes_total"
	//tcpMetric = "istio_requests_total"
	if !isIstioNamespace {
		// 1) query for traffic originating from "unknown" (i.e. the internet)
		tcpGroupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,response_flags", appLabel, verLabel, appLabel, verLabel)
		query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload="unknown",destination_workload_namespace="%s"} [%vs])) by (%s)`,
			tcpMetric,
			namespace,
			int(duration.Seconds()), // range duration for the query
			tcpGroupBy)
		tcpUnkVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMapTCP(trafficMap, &tcpUnkVector, o)

		// 2) query for traffic originating from a workload outside of the namespace. Exclude any "unknown" source telemetry (an unusual corner case)
		query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace!="%s",source_workload!="unknown",destination_service_namespace="%s"} [%vs])) by (%s)`,
			tcpMetric,
			namespace,
			namespace,
			int(duration.Seconds()), // range duration for the query
			tcpGroupBy)
		tcpExtVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMapTCP(trafficMap, &tcpExtVector, o)

		// 3) query for traffic originating from a workload inside of the namespace
		query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s"} [%vs])) by (%s)`,
			tcpMetric,
			namespace,
			int(duration.Seconds()), // range duration for the query
			tcpGroupBy)
		tcpInVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMapTCP(trafficMap, &tcpInVector, o)
	}

	return trafficMap
}

func populateTrafficMap(trafficMap graph.TrafficMap, vector *model.Vector, o graph.TelemetryOptions) {
	for _, s := range *vector {
		m := s.Metric
		lSourceWlNs, sourceWlNsOk := m["source_workload_namespace"]
		lSourceWl, sourceWlOk := m["source_workload"]
		lSourceApp, sourceAppOk := m[model.LabelName("source_"+appLabel)]
		lSourceVer, sourceVerOk := m[model.LabelName("source_"+verLabel)]
		lDestSvcNs, destSvcNsOk := m["destination_service_namespace"]
		lDestSvc, destSvcOk := m["destination_service"]
		lDestSvcName, destSvcNameOk := m["destination_service_name"]
		lDestWlNs, destWlNsOk := m["destination_workload_namespace"]
		lDestWl, destWlOk := m["destination_workload"]
		lDestApp, destAppOk := m[model.LabelName("destination_"+appLabel)]
		lDestVer, destVerOk := m[model.LabelName("destination_"+verLabel)]
		lProtocol, protocolOk := m["request_protocol"]
		lCode, codeOk := m["response_code"]
		lGrpc, grpcOk := m["grpc_response_status"]
		lFlags, flagsOk := m["response_flags"]

		if !sourceWlNsOk || !sourceWlOk || !sourceAppOk || !sourceVerOk || !destSvcNsOk || !destSvcOk || !destSvcNameOk || !destWlNsOk || !destWlOk || !destAppOk || !destVerOk || !protocolOk || !codeOk || !flagsOk {
			log.Warningf("Skipping %s, missing expected TS labels", m.String())
			continue
		}

		sourceWlNs := string(lSourceWlNs)
		sourceWl := string(lSourceWl)
		sourceApp := string(lSourceApp)
		sourceVer := string(lSourceVer)
		destSvc := string(lDestSvc)
		destWlNs := string(lDestWlNs)
		destWl := string(lDestWl)
		destApp := string(lDestApp)
		destVer := string(lDestVer)
		protocol := string(lProtocol)
		code := string(lCode)
		flags := string(lFlags)

		if util.IsBadSourceTelemetry(sourceWlNs, sourceWl, sourceApp) {
			continue
		}

		// set response code in a backward compatible way
		code = util.HandleResponseCode(protocol, code, grpcOk, string(lGrpc))

		// handle multicluster requests
		destSvcNs, destSvcName := util.HandleMultiClusterRequest(sourceWlNs, sourceWl, string(lDestSvcNs), string(lDestSvcName))

		if util.IsBadDestTelemetry(destSvc, destSvcName, destWl) {
			continue
		}

		// make code more readable by setting "host" because "destSvc" holds destination.service.host | request.host | "unknown"
		host := destSvc

		val := float64(s.Value)

		// don't inject a service node if destSvcName is not set or the dest node is already a service node.
		inject := false
		if o.InjectServiceNodes && graph.IsOK(destSvcName) {
			_, destNodeType := graph.Id(destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o.GraphType)
			inject = (graph.NodeTypeService != destNodeType)
		}
		if inject {
			addTraffic(trafficMap, val, protocol, code, flags, host, sourceWlNs, "", sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, "", "", "", "", o)
			addTraffic(trafficMap, val, protocol, code, flags, host, destSvcNs, destSvcName, "", "", "", destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)
		} else {
			addTraffic(trafficMap, val, protocol, code, flags, host, sourceWlNs, "", sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)
		}
	}
}

func addTraffic(trafficMap graph.TrafficMap, val float64, protocol, code, flags, host, sourceNs, sourceSvc, sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer string, o graph.TelemetryOptions) (source, dest *graph.Node) {
	source, sourceFound := addNode(trafficMap, sourceNs, sourceSvc, sourceNs, sourceWl, sourceApp, sourceVer, o)
	dest, destFound := addNode(trafficMap, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)

	addToDestServices(dest.Metadata, destSvcNs, destSvcName)

	var edge *graph.Edge
	for _, e := range source.Edges {
		if dest.ID == e.Dest.ID && e.Metadata[graph.ProtocolKey] == protocol {
			edge = e
			break
		}
	}
	if nil == edge {
		edge = source.AddEdge(dest)
		edge.Metadata[graph.ProtocolKey] = protocol
	}

	// A workload may mistakenly have multiple app and or version label values.
	// This is a misconfiguration we need to handle. See Kiali-1309.
	if sourceFound {
		handleMisconfiguredLabels(source, sourceApp, sourceVer, val, o)
	}
	if destFound {
		handleMisconfiguredLabels(dest, destApp, destVer, val, o)
	}
	graph.AddToMetadata(protocol, val, code, flags, host, source.Metadata, dest.Metadata, edge.Metadata)

	return source, dest
}

func populateTrafficMapTCP(trafficMap graph.TrafficMap, vector *model.Vector, o graph.TelemetryOptions) {
	for _, s := range *vector {
		m := s.Metric
		lSourceWlNs, sourceWlNsOk := m["source_workload_namespace"]
		lSourceWl, sourceWlOk := m["source_workload"]
		lSourceApp, sourceAppOk := m[model.LabelName("source_"+appLabel)]
		lSourceVer, sourceVerOk := m[model.LabelName("source_"+verLabel)]
		lDestSvcNs, destSvcNsOk := m["destination_service_namespace"]
		lDestSvc, destSvcOk := m["destination_service"]
		lDestSvcName, destSvcNameOk := m["destination_service_name"]
		lDestWlNs, destWlNsOk := m["destination_workload_namespace"]
		lDestWl, destWlOk := m["destination_workload"]
		lDestApp, destAppOk := m[model.LabelName("destination_"+appLabel)]
		lDestVer, destVerOk := m[model.LabelName("destination_"+verLabel)]
		lFlags, flagsOk := m["response_flags"]

		if !sourceWlNsOk || !sourceWlOk || !sourceAppOk || !sourceVerOk || !destSvcNsOk || !destSvcOk || !destSvcNameOk || !destWlNsOk || !destWlOk || !destAppOk || !destVerOk || !flagsOk {
			log.Warningf("Skipping %s, missing expected TS labels", m.String())
			continue
		}

		sourceWlNs := string(lSourceWlNs)
		sourceWl := string(lSourceWl)
		sourceApp := string(lSourceApp)
		sourceVer := string(lSourceVer)
		destSvc := string(lDestSvc)
		destWlNs := string(lDestWlNs)
		destWl := string(lDestWl)
		destApp := string(lDestApp)
		destVer := string(lDestVer)
		flags := string(lFlags)

		if util.IsBadSourceTelemetry(sourceWlNs, sourceWl, sourceApp) {
			continue
		}

		// handle multicluster requests
		destSvcNs, destSvcName := util.HandleMultiClusterRequest(sourceWlNs, sourceWl, string(lDestSvcNs), string(lDestSvcName))

		if util.IsBadDestTelemetry(destSvc, destSvcName, destWl) {
			continue
		}

		// make code more readable by setting "host" because "destSvc" holds destination.service.host | "unknown"
		host := destSvc

		val := float64(s.Value)

		// don't inject a service node if destSvcName is not set or the dest node is already a service node.
		inject := false
		if o.InjectServiceNodes && graph.IsOK(destSvcName) {
			_, destNodeType := graph.Id(destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o.GraphType)
			inject = (graph.NodeTypeService != destNodeType)
		}
		if inject {
			addTCPTraffic(trafficMap, val, flags, host, sourceWlNs, "", sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, "", "", "", "", o)
			addTCPTraffic(trafficMap, val, flags, host, destSvcNs, destSvcName, "", "", "", destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)
		} else {
			addTCPTraffic(trafficMap, val, flags, host, sourceWlNs, "", sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)
		}
	}
}

func addTCPTraffic(trafficMap graph.TrafficMap, val float64, flags, host, sourceNs, sourceSvc, sourceWl, sourceApp, sourceVer, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer string, o graph.TelemetryOptions) (source, dest *graph.Node) {
	source, sourceFound := addNode(trafficMap, sourceNs, sourceSvc, sourceNs, sourceWl, sourceApp, sourceVer, o)
	dest, destFound := addNode(trafficMap, destSvcNs, destSvcName, destWlNs, destWl, destApp, destVer, o)

	addToDestServices(dest.Metadata, destSvcNs, destSvcName)

	var edge *graph.Edge
	for _, e := range source.Edges {
		if dest.ID == e.Dest.ID && e.Metadata[graph.ProtocolKey] == "tcp" {
			edge = e
			break
		}
	}
	if nil == edge {
		edge = source.AddEdge(dest)
		edge.Metadata[graph.ProtocolKey] = "tcp"
	}

	// A workload may mistakenly have multiple app and or version label values.
	// This is a misconfiguration we need to handle. See Kiali-1309.
	if sourceFound {
		handleMisconfiguredLabels(source, sourceApp, sourceVer, val, o)
	}
	if destFound {
		handleMisconfiguredLabels(dest, destApp, destVer, val, o)
	}

	graph.AddToMetadata("tcp", val, "", flags, host, source.Metadata, dest.Metadata, edge.Metadata)

	return source, dest
}

func addToDestServices(md graph.Metadata, namespace, service string) {
	if !graph.IsOK(service) {
		return
	}
	destServices, ok := md[graph.DestServices]
	if !ok {
		destServices = graph.NewDestServicesMetadata()
		md[graph.DestServices] = destServices
	}
	destService := graph.ServiceName{Namespace: namespace, Name: service}
	destServices.(graph.DestServicesMetadata)[destService.Key()] = destService
}

func handleMisconfiguredLabels(node *graph.Node, app, version string, rate float64, o graph.TelemetryOptions) {
	isVersionedAppGraph := o.GraphType == graph.GraphTypeVersionedApp
	isWorkloadNode := node.NodeType == graph.NodeTypeWorkload
	isVersionedAppNode := node.NodeType == graph.NodeTypeApp && isVersionedAppGraph
	if isWorkloadNode || isVersionedAppNode {
		labels := []string{}
		if node.App != app {
			labels = append(labels, appLabel)
		}
		if node.Version != version {
			labels = append(labels, verLabel)
		}
		// prefer the labels of an active time series as often the other labels are inactive
		if len(labels) > 0 {
			node.Metadata[graph.IsMisconfigured] = fmt.Sprintf("labels=%v", labels)
			if rate > 0.0 {
				node.App = app
				node.Version = version
			}
		}
	}
}

func addNode(trafficMap graph.TrafficMap, serviceNs, service, workloadNs, workload, app, version string, o graph.TelemetryOptions) (*graph.Node, bool) {
	id, nodeType := graph.Id(serviceNs, service, workloadNs, workload, app, version, o.GraphType)
	node, found := trafficMap[id]
	if !found {
		namespace := workloadNs
		if !graph.IsOK(namespace) {
			namespace = serviceNs
		}
		newNode := graph.NewNodeExplicit(id, namespace, workload, app, version, service, nodeType, o.GraphType)
		node = &newNode
		trafficMap[id] = node
	}
	return node, found
}

// BuildNodeTrafficMap is required by the graph/TelemtryVendor interface
func BuildNodeTrafficMap(o graph.TelemetryOptions, client *prometheus.Client, globalInfo *graph.AppenderGlobalInfo) graph.TrafficMap {
	n := graph.NewNode(o.NodeOptions.Namespace, o.NodeOptions.Service, o.NodeOptions.Namespace, o.NodeOptions.Workload, o.NodeOptions.App, o.NodeOptions.Version, o.GraphType)

	log.Tracef("Build graph for node [%+v]", n)

	setLabels()
	appenders := appender.ParseAppenders(o)
	trafficMap := buildNodeTrafficMap(o.NodeOptions.Namespace, n, o, client)

	namespaceInfo := graph.NewAppenderNamespaceInfo(o.NodeOptions.Namespace)

	for _, a := range appenders {
		appenderTimer := internalmetrics.GetGraphAppenderTimePrometheusTimer(a.Name())
		a.AppendGraph(trafficMap, globalInfo, namespaceInfo)
		appenderTimer.ObserveDuration()
	}

	// The appenders can add/remove/alter nodes. After the manipulations are complete
	// we can make some final adjustments:
	// - mark the outsiders (i.e. nodes not in the requested namespaces)
	// - mark the traffic generators
	telemetry.MarkOutsideOrInaccessible(trafficMap, o)
	telemetry.MarkTrafficGenerators(trafficMap)

	// Note that this is where we would call reduceToServiceGraph for graphTypeService but
	// the current decision is to not reduce the node graph to provide more detail.  This may be
	// confusing to users, we'll see...

	return trafficMap
}

// buildNodeTrafficMap returns a map of all nodes requesting or requested by the target node (key=id). Node graphs
// are from the perspective of the node, as such we use destination telemetry for incoming traffic and source telemetry
// for outgoing traffic.
func buildNodeTrafficMap(namespace string, n graph.Node, o graph.TelemetryOptions, client *prometheus.Client) graph.TrafficMap {
	httpMetric := "istio_requests_total"
	interval := o.Namespaces[namespace].Duration
	// create map to aggregate traffic by response code
	trafficMap := graph.NewTrafficMap()

	// query prometheus for request traffic in two queries:
	// 1) query for incoming traffic
	var query string
	// For an Istio node limit incoming traffic to only the requested Istio namespaces.
	var sourceWorkloadQuery = ""
	if o.Namespaces[namespace].IsIstio {
		excludedIstioNamespaces := config.GetIstioNamespaces(o.Namespaces.GetIstioNamespaces())
		if len(excludedIstioNamespaces) > 0 {
			excludedIstioRegex := strings.Join(excludedIstioNamespaces, "|")
			sourceWorkloadQuery = fmt.Sprintf(`,source_workload_namespace!~"%s"`, excludedIstioRegex)
		}
	}
	groupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,request_protocol,response_code,grpc_response_status,response_flags", appLabel, verLabel, appLabel, verLabel)
	switch n.NodeType {
	case graph.NodeTypeWorkload:
		query = fmt.Sprintf(`sum(rate(%s{reporter="destination"%s,destination_workload_namespace="%s",destination_workload="%s"} [%vs])) by (%s)`,
			httpMetric,
			sourceWorkloadQuery,
			namespace,
			n.Workload,
			int(interval.Seconds()), // range duration for the query
			groupBy)
	case graph.NodeTypeApp:
		if graph.IsOK(n.Version) {
			query = fmt.Sprintf(`sum(rate(%s{reporter="destination",%sdestination_service_namespace="%s",destination_%s="%s",destination_%s="%s"} [%vs])) by (%s)`,
				httpMetric,
				sourceWorkloadQuery,
				namespace,
				appLabel,
				n.App,
				verLabel,
				n.Version,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		} else {
			query = fmt.Sprintf(`sum(rate(%s{reporter="destination"%s,destination_service_namespace="%s",destination_%s="%s"} [%vs])) by (%s)`,
				httpMetric,
				sourceWorkloadQuery,
				namespace,
				appLabel,
				n.App,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		}
	case graph.NodeTypeService:
		// for service requests we want source reporting to capture source-reported errors.  But unknown only generates destination telemetry.  So
		// perform a special query just to capture [successful] request telemetry from unknown.
		query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload="unknown",destination_workload_namespace="%s",destination_service_name=~"%s|%s\\..+\\.global"} [%vs])) by (%s)`,
			httpMetric,
			namespace,
			n.Service,
			n.Service,
			int(interval.Seconds()), // range duration for the query
			groupBy)
		vector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMap(trafficMap, &vector, o)

		query = fmt.Sprintf(`sum(rate(%s{reporter="source",destination_service_namespace="%s",destination_service_name=~"%s|%s\\..+\\.global"} [%vs])) by (%s)`,
			httpMetric,
			namespace,
			n.Service,
			n.Service,
			int(interval.Seconds()), // range duration for the query
			groupBy)
	default:
		graph.Error(fmt.Sprintf("NodeType [%s] not supported", n.NodeType))
	}
	inVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
	populateTrafficMap(trafficMap, &inVector, o)

	// 2) query for outbound traffic
	switch n.NodeType {
	case graph.NodeTypeWorkload:
		query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_workload="%s"} [%vs])) by (%s)`,
			httpMetric,
			namespace,
			n.Workload,
			int(interval.Seconds()), // range duration for the query
			groupBy)
	case graph.NodeTypeApp:
		if graph.IsOK(n.Version) {
			query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_%s="%s",source_%s="%s"} [%vs])) by (%s)`,
				httpMetric,
				namespace,
				appLabel,
				n.App,
				verLabel,
				n.Version,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		} else {
			query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_%s="%s"} [%vs])) by (%s)`,
				httpMetric,
				namespace,
				appLabel,
				n.App,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		}
	case graph.NodeTypeService:
		query = ""
	default:
		graph.Error(fmt.Sprintf("NodeType [%s] not supported", n.NodeType))
	}
	outVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
	populateTrafficMap(trafficMap, &outVector, o)

	// istio component telemetry is only reported destination-side, so we must perform additional queries

	if config.IsIstioNamespace(namespace) {
		// find traffic from the source istio namespace to any of the requested istio namespaces
		istioNamespacesRegex := strings.Join(o.Namespaces.GetIstioNamespaces(), "|")

		// 2a) supplemental query for istio-to-istio traffic
		switch n.NodeType {
		case graph.NodeTypeWorkload:
			query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload_namespace="%s",source_workload="%s",destination_service_namespace=~"%s"} [%vs])) by (%s)`,
				httpMetric,
				namespace,
				n.Workload,
				istioNamespacesRegex,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		case graph.NodeTypeApp:
			if graph.IsOK(n.Version) {
				query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload_namespace="%s",source_%s="%s",source_%s="%s",destination_service_namespace=~"%s"} [%vs])) by (%s)`,
					httpMetric,
					namespace,
					appLabel,
					n.App,
					verLabel,
					n.Version,
					istioNamespacesRegex,
					int(interval.Seconds()), // range duration for the query
					groupBy)
			} else {
				query = fmt.Sprintf(`sum(rate(%s{reporter="destination",source_workload_namespace="%s",source_%s="%s",destination_service_namespace=~"%s"} [%vs])) by (%s)`,
					httpMetric,
					namespace,
					appLabel,
					n.App,
					istioNamespacesRegex,
					int(interval.Seconds()), // range duration for the query
					groupBy)
			}
		case graph.NodeTypeService:
			query = fmt.Sprintf(`sum(rate(%s{reporter="destination",destination_service_namespace="%s",destination_service_name="%s"} [%vs])) by (%s)`,
				httpMetric,
				namespace,
				n.Service,
				int(interval.Seconds()), // range duration for the query
				groupBy)
		default:
			graph.Error(fmt.Sprintf("NodeType [%s] not supported", n.NodeType))
		}
		outIstioVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMap(trafficMap, &outIstioVector, o)
	}

	// Section for TCP services, note there is no TCP Istio traffic
	if !config.IsIstioNamespace(namespace) {
		tcpMetric := "istio_tcp_sent_bytes_total"

		tcpGroupBy := fmt.Sprintf("source_workload_namespace,source_workload,source_%s,source_%s,destination_service_namespace,destination_service,destination_service_name,destination_workload_namespace,destination_workload,destination_%s,destination_%s,response_flags", appLabel, verLabel, appLabel, verLabel)
		switch n.NodeType {
		case graph.NodeTypeWorkload:
			query = fmt.Sprintf(`sum(rate(%s{reporter="source",destination_workload_namespace="%s",destination_workload="%s"} [%vs])) by (%s)`,
				tcpMetric,
				namespace,
				n.Workload,
				int(interval.Seconds()), // range duration for the query
				tcpGroupBy)
		case graph.NodeTypeApp:
			if graph.IsOK(n.Version) {
				query = fmt.Sprintf(`sum(rate(%s{reporter="source",destination_service_namespace="%s",destination_%s="%s",destination_%s="%s"} [%vs])) by (%s)`,
					tcpMetric,
					namespace,
					appLabel,
					n.App,
					verLabel,
					n.Version,
					int(interval.Seconds()), // range duration for the query
					tcpGroupBy)
			} else {
				query = fmt.Sprintf(`sum(rate(%s{reporter="source",destination_service_namespace="%s",destination_%s="%s"} [%vs])) by (%s)`,
					tcpMetric,
					namespace,
					appLabel,
					n.App,
					int(interval.Seconds()), // range duration for the query
					tcpGroupBy)
			}
		case graph.NodeTypeService:
			// TODO: Do we need to handle requests from unknown in a special way (like in HTTP above)? Not sure how tcp is reported from unknown.
			query = fmt.Sprintf(`sum(rate(%s{reporter="source",destination_service_namespace="%s",destination_service_name="%s"} [%vs])) by (%s)`,
				tcpMetric,
				namespace,
				n.Service,
				int(interval.Seconds()), // range duration for the query
				tcpGroupBy)
		default:
			graph.Error(fmt.Sprintf("NodeType [%s] not supported", n.NodeType))
		}
		tcpInVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMapTCP(trafficMap, &tcpInVector, o)

		// 2) query for outbound traffic
		switch n.NodeType {
		case graph.NodeTypeWorkload:
			query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_workload="%s"} [%vs])) by (%s)`,
				tcpMetric,
				namespace,
				n.Workload,
				int(interval.Seconds()), // range duration for the query
				tcpGroupBy)
		case graph.NodeTypeApp:
			if graph.IsOK(n.Version) {
				query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_%s="%s",source_%s="%s"} [%vs])) by (%s)`,
					tcpMetric,
					namespace,
					appLabel,
					n.App,
					verLabel,
					n.Version,
					int(interval.Seconds()), // range duration for the query
					tcpGroupBy)
			} else {
				query = fmt.Sprintf(`sum(rate(%s{reporter="source",source_workload_namespace="%s",source_%s="%s"} [%vs])) by (%s)`,
					tcpMetric,
					namespace,
					appLabel,
					n.App,
					int(interval.Seconds()), // range duration for the query
					tcpGroupBy)
			}
		case graph.NodeTypeService:
			query = ""
		default:
			graph.Error(fmt.Sprintf("NodeType [%s] not supported", n.NodeType))
		}
		tcpOutVector := promQuery(query, time.Unix(o.QueryTime, 0), client.API())
		populateTrafficMapTCP(trafficMap, &tcpOutVector, o)
	}

	return trafficMap
}

func promQuery(query string, queryTime time.Time, api prom_v1.API) model.Vector {
	if query == "" {
		return model.Vector{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// wrap with a round() to be in line with metrics api
	query = fmt.Sprintf("round(%s,0.001)", query)
	log.Tracef("Graph query:\n%s@time=%v (now=%v, %v)\n", query, queryTime.Format(graph.TF), time.Now().Format(graph.TF), queryTime.Unix())

	promtimer := internalmetrics.GetPrometheusProcessingTimePrometheusTimer("Graph-Generation")
	value, err := api.Query(ctx, query, queryTime)
	graph.CheckError(err)
	promtimer.ObserveDuration() // notice we only collect metrics for successful prom queries

	switch t := value.Type(); t {
	case model.ValVector: // Instant Vector
		return value.(model.Vector)
	default:
		graph.Error(fmt.Sprintf("No handling for type %v!\n", t))
	}

	return nil
}
