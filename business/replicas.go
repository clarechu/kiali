package business

import (
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/prometheus/internalmetrics"
	"time"
)

type ReplicaseService struct {
	prom          prometheus.ClientInterface
	k8s           kubernetes.IstioClientInterface
	businessLayer *Layer
}

func (in *ReplicaseService) GetWorkloadList(namespace string) (models.WorkloadList, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "WorkloadService", "GetWorkloadList")
	defer promtimer.ObserveNow(&err)

	workloadList := &models.WorkloadList{
		Namespace: models.Namespace{Name: namespace, CreationTimestamp: time.Time{}},
		Workloads: []models.WorkloadListItem{},
	}
	ws, err := fetchReplicas(in.businessLayer, namespace, "")
	if err != nil {
		return *workloadList, err
	}

	for _, w := range ws {
		wItem := &models.WorkloadListItem{}
		wItem.ParseWorkload(w)
		workloadList.Workloads = append(workloadList.Workloads, *wItem)
	}

	return *workloadList, nil
}

//todo 1. Replicas
// health 查询节点是都正常
// 响应时间 查询服务的响应时间
//fixme isMTLS 加锁 这个暂时不做 明天在搞

//todo 下次在写 。。。 暂时就这个样子 😄😄😄
func fetchReplicas(layer *Layer, namespace string, labelSelector string) (list models.Workloads, err error) {
	return fetchWorkloads(layer, namespace, labelSelector)
}
