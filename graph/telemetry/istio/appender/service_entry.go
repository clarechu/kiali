package appender

import (
	"errors"
	"github.com/kiali/kiali/prometheus"
	"strings"
	"time"

	"github.com/kiali/kiali/business"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
)

const ServiceEntryAppenderName = "serviceEntry"

// ServiceEntryAppender is responsible for identifying service nodes that are defined in Istio as
// a serviceEntry. A single serviceEntry can define multiple hosts and as such multiple service nodes may
// map to different hosts of a single serviceEntry. We'll call these "se-service" nodes.  The appender
// handles this in the following way:
//   For Each "se-service" node
//      if necessary, create an aggregate serviceEntry node ("se-aggregate")
//        -- an "se-aggregate" is a service node with isServiceEntry set in the metadata
//        -- an "se-aggregate" is namespace-specific. This can lead to mutiple serviceEntry nodes
//           in a multi-namespace graph. This makes some sense because serviceEntries are "exported"
//           to individual namespaces.
//      aggregate the "se-service" node into the "se-aggregate" node
//        -- incoming edges
//        -- outgoing edges (unusual but can have outgoing edge to egress gateway)
//        -- per-host traffic (in the metadata)
//      remove the "se-service" node from the trafficMap
//      add any new "se-aggregate" node to the trafficMap
//
// Doc Links
// - https://istio.io/docs/reference/config/networking/v1alpha3/service-entry/#ServiceEntry
// - https://istio.io/docs/examples/advanced-gateways/wildcard-egress-hosts/
//
// A note about wildcard hosts. External service entries allow for prefix wildcarding such that
// many different service requests may be handled by the same service entry definition.  For example,
// host = *.wikipedia.com would match requests for en.wikipedia.com and de.wikipedia.com. The Istio
// telemetry produces only one "se-service" node with the wilcard host as the destination_service_name.
//这个有点复杂 主要负责
type ServiceEntryAppender struct {
	AccessibleNamespaces map[string]time.Time
	GraphType            string // This appender does not operate on service graphs because it adds workload nodes.
}

// Name implements Appender
func (a ServiceEntryAppender) Name() string {
	return ServiceEntryAppenderName
}

// AppendGraph implements Appender
func (a ServiceEntryAppender) AppendGraph(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo) error {
	if len(trafficMap) == 0 {
		return errors.New("trafficMap is nil")
	}

	a.applyServiceEntries(trafficMap, globalInfo, namespaceInfo)
	return nil
}

// aggregateEdges identifies edges that are going from <node> to <serviceEntryNode> and
// aggregates them in only one edge per protocol. This ensures that the traffic map
// will comply with the assumption/rule of one edge per protocol between any two nodes.
func aggregateEdges(node *graph.Node, serviceEntryNode *graph.Node) {
	edgesToAggregate := make(map[string][]*graph.Edge)
	bound := 0
	for _, edge := range node.Edges {
		if edge.Dest == serviceEntryNode {
			protocol := edge.Metadata[graph.ProtocolKey].(string)
			edgesToAggregate[protocol] = append(edgesToAggregate[protocol], edge)
		} else {
			// Manipulating the slice as in this StackOverflow post: https://stackoverflow.com/a/20551116
			node.Edges[bound] = edge
			bound++
		}
	}
	node.Edges = node.Edges[:bound]
	// Add aggregated edge
	for protocol, edges := range edgesToAggregate {
		aggregatedEdge := node.AddEdge(serviceEntryNode)
		aggregatedEdge.Metadata[graph.ProtocolKey] = protocol
		for _, e := range edges {
			graph.AggregateEdgeTraffic(e, aggregatedEdge)
		}
	}
}

func (a ServiceEntryAppender) applyServiceEntries(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo) {
	// a map of "se-service" nodes to the "se-aggregate" information
	seMap := make(map[*serviceEntry][]*graph.Node)
	// 找到所有的service entry node 节点
	for _, n := range trafficMap {
		// only a service node can be a service entry
		if n.NodeType != graph.NodeTypeService {
			continue
		}
		// PassthroughCluster or BlackHoleCluster is not a service entry (nor a defined service)
		if n.Metadata[graph.IsEgressCluster] == true {
			continue
		}
		// a serviceEntry has at most one outgoing edge, to an egress gateway. (note: it may be that it
		// can only lead to "istio-egressgateway" but at the time of writing we're not sure, and so don't
		// want to hardcode that assumption.)
		if len(n.Edges) > 1 {
			continue
		}

		// A service node represents a serviceEntry when the service name matches serviceEntry host. Map
		// these "se-service" nodes to the serviceEntries that represent them.
		if se, ok := a.getServiceEntry(n.Service, globalInfo); ok {
			if nodes, ok := seMap[se]; ok {
				seMap[se] = append(nodes, n)
			} else {
				seMap[se] = []*graph.Node{n}
			}
		}
	}

	// Replace "se-service" nodes with an "se-aggregate" serviceEntry node
	// 如果有 service entry
	for se, seServiceNodes := range seMap {
		serviceEntryNode := graph.NewNode(namespaceInfo.Namespace, se.name, "", "", "", "", a.GraphType)
		serviceEntryNode.Metadata[graph.IsServiceEntry] = se.location
		serviceEntryNode.Metadata[graph.DestServices] = graph.NewDestServicesMetadata()
		// 以上是新建一个node
		for _, doomedSeServiceNode := range seServiceNodes {
			// aggregate node traffic
			//doomedSeServiceNode 一个SeServiceNode 节点
			graph.AggregateNodeTraffic(doomedSeServiceNode, &serviceEntryNode)
			// aggregate node dest-services to capture all of the distinct requested services
			if destServices, ok := doomedSeServiceNode.Metadata[graph.DestServices]; ok {
				for k, v := range destServices.(graph.DestServicesMetadata) {
					serviceEntryNode.Metadata[graph.DestServices].(graph.DestServicesMetadata)[k] = v
				}
			}
			// redirect edges leading to the doomed se-service node to the new aggregate
			// 将导致注定失败的se-service节点的边缘重定向到新的聚合
			for _, n := range trafficMap {
				for _, edge := range n.Edges {
					if edge.Dest.ID == doomedSeServiceNode.ID {
						edge.Dest = &serviceEntryNode
					}
				}

				// If there is more than one doomed node, edges leading to the new aggregated node must
				// also be aggregated per source and protocol.
				if len(seServiceNodes) > 1 {
					aggregateEdges(n, &serviceEntryNode)
				}
			}
			// redirect/aggregate edges leading from the doomed se-service node [to an egress gateway]
			for _, doomedEdge := range doomedSeServiceNode.Edges {
				var aggregateEdge *graph.Edge
				for _, e := range serviceEntryNode.Edges {
					if doomedEdge.Dest.ID == e.Dest.ID && doomedEdge.Metadata[graph.ProtocolKey] == e.Metadata[graph.ProtocolKey] {
						aggregateEdge = e
						break
					}
				}
				if nil == aggregateEdge {
					aggregateEdge = serviceEntryNode.AddEdge(doomedEdge.Dest)
					aggregateEdge.Metadata[graph.ProtocolKey] = doomedEdge.Metadata[graph.ProtocolKey]
				}
				graph.AggregateEdgeTraffic(doomedEdge, aggregateEdge)
			}
			delete(trafficMap, doomedSeServiceNode.ID)
		}
		trafficMap[serviceEntryNode.ID] = &serviceEntryNode
	}
}

// getServiceEntry queries the cluster API to resolve service entries across all accessible namespaces
// in the cluster.
// TODO: I don't know what happens (nothing good) if a ServiceEntry is defined in an inaccessible namespace but exported to
// all namespaces (exportTo: *). It's possible that would allow traffic to flow from an accessible workload
// through a serviceEntry whose definition we can't fetch.
func (a ServiceEntryAppender) getServiceEntry(serviceName string, globalInfo *graph.AppenderGlobalInfo) (*serviceEntry, bool) {
	serviceEntryHosts, found := getServiceEntryHosts(globalInfo)
	if !found {
		for ns := range a.AccessibleNamespaces {
			//todo cache
			istioCfg, err := globalInfo.Business.IstioConfig.GetIstioConfigList(business.IstioConfigCriteria{
				IncludeServiceEntries: true,
				Namespace:             ns,
			})
			graph.CheckError(err)

			for _, entry := range istioCfg.ServiceEntries {
				if entry.Spec.Hosts != nil {
					location := "MESH_EXTERNAL"
					if entry.Spec.Location == "MESH_INTERNAL" {
						location = "MESH_INTERNAL"
					}
					se := serviceEntry{
						location: location,
						address:  entry.Spec.Endpoints,
						name:     entry.Metadata.Name,
					}
					for _, host := range entry.Spec.Hosts.([]interface{}) {
						serviceEntryHosts.addHost(host.(string), &se)
					}
				}
			}
		}
		globalInfo.Vendor[serviceEntryHostsKey] = serviceEntryHosts
	}

	for host, se := range serviceEntryHosts {
		// handle exact match
		// note: this also handles wildcard-prefix cases because the destination_service_name set by istio
		// is the matching host (e.g. *.wikipedia.com), not the rested service (e.g. de.wikipedia.com)
		if host == serviceName {
			return se, true
		}
		// handle serviceName prefix (e.g. host = serviceName.namespace.svc.cluster.local)
		if se.location == "MESH_INTERNAL" {
			hostSplitted := strings.Split(host, ".")

			if len(hostSplitted) == 3 && hostSplitted[2] == config.IstioMultiClusterHostSuffix {
				// If suffix is "global", this node should be a service entry
				// related to multi-cluster configs. Only exact match should be done, so
				// skip prefix matching.
				//
				// Number of entries == 3 in the host is checked because the host
				// must be of the form svc.namespace.global for Istio to
				// work correctly in the multi-cluster/multiple-control-plane scenario.
				continue
			} else if hostSplitted[0] == serviceName {
				return se, true
			}
		}
	}

	return nil, false
}

func (a ServiceEntryAppender) AppendGraphNoAuth(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo, client *prometheus.Client) {

}
