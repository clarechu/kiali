package appender

import (
	"errors"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
)

//ReplicasNodeAppenderName 插入 工作负载数量
const ReplicasNodeAppenderName = "replicasNode"

type ReplicasNodeAppender struct {
}

// Name implements Appender
func (r ReplicasNodeAppender) Name() string {
	return ReplicasNodeAppenderName
}

func (r ReplicasNodeAppender) AppendGraph(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo) error {
	if len(trafficMap) == 0 {
		return errors.New("trafficMap is nil")
	}

	if getWorkloadList(namespaceInfo) == nil {
		workloadList, err := globalInfo.Business.Replicase.GetWorkloadList(namespaceInfo.Namespace)
		if err != nil {
			return err
		}
		namespaceInfo.Vendor[workloadListKey] = &workloadList
	}

	r.applyNodes(trafficMap, namespaceInfo)
	return nil
}

func (r ReplicasNodeAppender) applyNodes(trafficMap graph.TrafficMap, namespaceInfo *graph.AppenderNamespaceInfo) {
	workloadList := namespaceInfo.Vendor[workloadListKey].(*models.WorkloadList)
	for _, workload := range workloadList.Workloads {
		workloadId, _ := graph.Id("", "", workloadList.Namespace.Name, workload.Name, workload.Labels["app"], workload.Labels["version"], graph.GraphTypeVersionedApp)
		serviceId, _ := graph.Id(workloadList.Namespace.Name, workload.Labels["app"], "", "", workload.Labels["app"], workload.Labels["version"], graph.GraphTypeService)
		log.Debugf("workload :%v", workload.Name)
		var traffic *graph.Node
		if g, exit := trafficMap[workloadId]; exit {
			traffic = g
			traffic.IsHealth = true
			traffic.IstioSidecar = workload.IstioSidecar
			traffic.Replicas = workload.PodCount
			traffic.Annotations = workload.Annotations
			traffic.Labels = workload.Labels
		}
		if g, exit := trafficMap[serviceId]; exit {
			traffic = g
			traffic.IsHealth = true
			traffic.IstioSidecar = workload.IstioSidecar
			traffic.Replicas = workload.PodCount
			traffic.Annotations = workload.Annotations
			traffic.Labels = workload.Labels
		}

	}
}

func (r ReplicasNodeAppender) AppendGraphNoAuth(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo, client *prometheus.Client) {

}
