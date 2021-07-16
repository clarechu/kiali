package appender

import (
	"errors"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/prometheus"
)

const SidecarsCheckAppenderName = "sidecarsCheck"

// SidecarsCheckAppender flags nodes whose backing workloads are missing at least one Envoy sidecar. Note that
// a node with no backing workloads is not flagged.
// Name: sidecarsCheck
// SidecarsCheckAppender标记其后备工作负载缺少至少一个Envoy sidecar的节点。请注意，
// 没有后备工作负载的节点未标记。
// 名称：sidecarsCheck
type SidecarsCheckAppender struct{}

// Name implements Appender
func (a SidecarsCheckAppender) Name() string {
	return SidecarsCheckAppenderName
}

// AppendGraph implements Appender
func (a SidecarsCheckAppender) AppendGraph(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo) error {
	if len(trafficMap) == 0 {
		return errors.New("trafficMap is nil")
	}

	if getWorkloadList(namespaceInfo) == nil {
		workloadList, err := globalInfo.Business.Workload.GetWorkloadList(namespaceInfo.Namespace)
		if err != nil {
			return err
		}
		namespaceInfo.Vendor[workloadListKey] = &workloadList
	}

	a.applySidecarsChecks(trafficMap, namespaceInfo)
	return nil
}

func (a *SidecarsCheckAppender) applySidecarsChecks(trafficMap graph.TrafficMap, namespaceInfo *graph.AppenderNamespaceInfo) {
	for _, n := range trafficMap {
		// Skip the check if this node is outside the requested namespace, we limit badging to the requested namespaces
		if n.Namespace != namespaceInfo.Namespace {
			continue
		}

		// We whitelist istio components because they may not report telemetry using injected sidecars.
		if config.IsIstioNamespace(n.Namespace) {
			continue
		}

		// dead nodes tell no tales (er, have no pods)
		if isDead, ok := n.Metadata[graph.IsDead]; ok && isDead.(bool) {
			continue
		}

		// get the workloads for the node and check to see if they have sidecars. Note that
		// if there are no workloads/pods we don't flag it as missing sidecars.  No pods means
		// no missing sidecars.  (In most cases this means it was flagged as dead, and handled above)
		hasIstioSidecar := true
		switch n.NodeType {
		case graph.NodeTypeWorkload:
			if workload, found := getWorkload(n.Workload, namespaceInfo); found {
				hasIstioSidecar = workload.IstioSidecar
			}
		case graph.NodeTypeApp:
			workloads := getAppWorkloads(n.App, n.Version, namespaceInfo)
			if len(workloads) > 0 {
				for _, workload := range workloads {
					if !workload.IstioSidecar {
						hasIstioSidecar = false
						break
					}
				}
			}
		default:
			continue
		}

		if !hasIstioSidecar {
			n.Metadata[graph.HasMissingSC] = true
		}
	}
}

func (a SidecarsCheckAppender) AppendGraphNoAuth(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo, client *prometheus.Client) {

}
