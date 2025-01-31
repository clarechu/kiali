package appender

import (
	"errors"
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/graph"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
	"strings"
)

const UnusedNodeAppenderName = "unusedNode"

// UnusedNodeAppender looks for services that have never seen request traffic.  It adds nodes to represent the
// unused definitions.  The added node types depend on the graph type and/or labeling on the definition.
// Name: unusedNode
// GetGoFunctionMetric返回一个Success或FailureMetricType对象，该对象可用于存储
// 调用函数成功时，函数处理时间指标的持续时间值。
// 如果不成功，则递增失败计数器。
// 如果围棋函数不在一个类型上（即是一个全局函数），请为goType传入一个空字符串。
// 当该函数返回时，定时器立即开始计时。
// 请参阅 SuccessOrFailureMetricType 的注释，了解如何使用返回的对象。
type UnusedNodeAppender struct {
	GraphType          string
	InjectServiceNodes bool // This appender addes unused services only when service node are injected or graphType=service
	IsNodeGraph        bool // This appender does not operate on node detail graphs because we want to focus on the specific node.
}

// Name implements Appender
func (a UnusedNodeAppender) Name() string {
	return UnusedNodeAppenderName
}

// AppendGraph implements Appender
func (a UnusedNodeAppender) AppendGraph(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo) error {
	if a.IsNodeGraph {
		return errors.New("trafficMap is nil")
	}

	services := []models.ServiceDetails{}
	workloads := []models.WorkloadListItem{}

	if a.GraphType != graph.GraphTypeService {
		if getWorkloadList(namespaceInfo) == nil {
			workloadList, err := globalInfo.Business.Workload.GetWorkloadList(namespaceInfo.Namespace)
			graph.CheckError(err)
			namespaceInfo.Vendor[workloadListKey] = &workloadList
		}
		workloads = getWorkloadList(namespaceInfo).Workloads
	}

	if a.GraphType == graph.GraphTypeService || a.InjectServiceNodes {
		if getServiceDefinitionList(namespaceInfo) == nil {
			sdl, err := globalInfo.Business.Svc.GetServiceDefinitionList(namespaceInfo.Namespace)
			if err != nil {
				return err
			}
			namespaceInfo.Vendor[serviceDefinitionListKey] = sdl
		}
		services = getServiceDefinitionList(namespaceInfo).ServiceDefinitions
	}

	a.addUnusedNodes(trafficMap, namespaceInfo.Namespace, services, workloads)
	// 删除不要的节点
	if a.GraphType == graph.GraphTypeVersionedApp {
		a.deleteUnusedNodes(trafficMap, namespaceInfo.Namespace, services, workloads)
	} else {
		a.deleteServiceUnusedNodes(trafficMap, namespaceInfo.Namespace, services, workloads)
	}
	return nil
}

func (a UnusedNodeAppender) deleteUnusedNodes(trafficMap graph.TrafficMap, namespace string, services []models.ServiceDetails, workloads []models.WorkloadListItem) {
	traffic := make(map[string]interface{}, 0)
	unknownTraffic := make(map[string]interface{}, 0)
	for k := range trafficMap {
		if !strings.Contains(k, "unknown") {
			traffic[k] = 0
		} else {
			if k == "unknown_source" {
				unknownTraffic[k] = 0
			}
		}
	}

	/*	for k := range trafficMap {
		if !strings.Contains(k, "unknown") && !strings.Contains(k, "istio-ingressgateway") {
			traffic[k] = 0
		}
	}*/

	for _, svc := range services {
		graphId, _ := graph.Id(namespace, svc.Service.Name, "", "", "", "", graph.GraphTypeService)
		delete(traffic, graphId)
	}
	for _, workload := range workloads {
		graphId, _ := graph.Id("", "", namespace, workload.Name, workload.Labels["app"], workload.Labels["version"], graph.GraphTypeVersionedApp)
		delete(traffic, graphId)
	}

	for k := range traffic {
		delete(trafficMap, k)
	}

	for k := range unknownTraffic {
		delete(trafficMap, k)
	}

	// fixme 下面那种写法导致数组越界了 现在测试一下看看可不可以 有没有用
	for k := range traffic {
		for _, tv := range trafficMap {
			for i := 0; i < len(tv.Edges); {
				if tv.Edges[i].Source.ID == k || tv.Edges[i].Dest.ID == k {
					tv.Edges = append(tv.Edges[:i], tv.Edges[i+1:]...)
				} else {
					i++
				}
			}
		}
	}
}

func (a UnusedNodeAppender) deleteServiceUnusedNodes(trafficMap graph.TrafficMap, namespace string, services []models.ServiceDetails, workloads []models.WorkloadListItem) {
	for k := range trafficMap {
		if k == "unknown_source" {
			delete(trafficMap, k)
		}
	}
}

func (a UnusedNodeAppender) addUnusedNodes(trafficMap graph.TrafficMap, namespace string, services []models.ServiceDetails, workloads []models.WorkloadListItem) {
	unusedTrafficMap := a.buildUnusedTrafficMap(trafficMap, namespace, services, workloads)

	// Integrate the unused nodes into the existing traffic map
	for id, unusedNode := range unusedTrafficMap {
		trafficMap[id] = unusedNode
	}
}

func (a UnusedNodeAppender) buildUnusedTrafficMap(trafficMap graph.TrafficMap, namespace string, services []models.ServiceDetails, workloads []models.WorkloadListItem) graph.TrafficMap {
	unusedTrafficMap := graph.NewTrafficMap()

	for _, s := range services {
		id, nodeType := graph.Id(namespace, s.Service.Name, "", "", "", "", a.GraphType)
		if _, found := trafficMap[id]; !found {
			if _, found = unusedTrafficMap[id]; !found {
				log.Tracef("Adding unused node for service [%s]", s.Service.Name)

				node := graph.NewNodeExplicit(id, namespace, "", "", "", s.Service.Name, nodeType, a.GraphType)
				// note: we don't know what the protocol really should be, http is most common, it's a dead edge anyway
				node.Metadata = graph.Metadata{"httpIn": 0.0, "httpOut": 0.0, "isUnused": true}
				unusedTrafficMap[id] = &node
			}
		}
	}

	cfg := config.Get()
	appLabel := cfg.IstioLabels.AppLabelName
	versionLabel := cfg.IstioLabels.VersionLabelName
	for _, w := range workloads {
		labels := w.Labels
		app := graph.Unknown
		version := graph.Unknown
		if v, ok := labels[appLabel]; ok {
			app = v
		}
		if v, ok := labels[versionLabel]; ok {
			version = v
		}
		id, nodeType := graph.Id("", "", namespace, w.Name, app, version, a.GraphType)
		if _, found := trafficMap[id]; !found {
			if _, found = unusedTrafficMap[id]; !found {
				log.Tracef("Adding unused node for workload [%s] with labels [%v]", w.Name, labels)
				node := graph.NewNodeExplicit(id, namespace, w.Name, app, version, "", nodeType, a.GraphType)
				// note: we don't know what the protocol really should be, http is most common, it's a dead edge anyway
				node.Metadata = graph.Metadata{"httpIn": 0.0, "httpOut": 0.0, "isUnused": true}
				unusedTrafficMap[id] = &node
			}
		}
	}
	return unusedTrafficMap
}

func (a UnusedNodeAppender) AppendGraphNoAuth(trafficMap graph.TrafficMap, globalInfo *graph.AppenderGlobalInfo, namespaceInfo *graph.AppenderNamespaceInfo, client *prometheus.Client) {

}
