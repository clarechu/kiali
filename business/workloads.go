package business

import (
	errors2 "errors"
	kmodel "github.com/kiali/k-charted/model"
	osapps_v1 "github.com/openshift/api/apps/v1"
	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	batch_v1beta1 "k8s.io/api/batch/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sort"
	"sync"
	"time"

	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/models"
	"github.com/kiali/kiali/prometheus"
	"github.com/kiali/kiali/prometheus/internalmetrics"
	"k8s.io/apimachinery/pkg/api/errors"
)

// Workload deals with fetching istio/kubernetes workloads related content and convert to kiali model
type WorkloadService struct {
	prom          prometheus.ClientInterface
	k8s           kubernetes.IstioClientInterface
	businessLayer *Layer
}

var (
	excludedWorkloads map[string]bool
)

func isWorkloadIncluded(workload string) bool {
	if excludedWorkloads == nil {
		return true
	}
	return !excludedWorkloads[workload]
}

// GetWorkloadList is the API handler to fetch the list of workloads in a given namespace.
// 查找所有 workload 工作负载
func (in *WorkloadService) GetWorkloadList(namespace string) (models.WorkloadList, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "WorkloadService", "GetWorkloadList")
	defer promtimer.ObserveNow(&err)

	workloadList := &models.WorkloadList{
		Namespace: models.Namespace{Name: namespace, CreationTimestamp: time.Time{}},
		Workloads: []models.WorkloadListItem{},
	}
	ws, err := fetchWorkloads(in.businessLayer, namespace, "")
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

// GetWorkload is the API handler to fetch details of a specific workload.
// If includeServices is set true, the Workload will fetch all services related
func (in *WorkloadService) GetWorkload(namespace string, workloadName string, includeServices bool) (*models.Workload, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "WorkloadService", "GetWorkload")
	defer promtimer.ObserveNow(&err)

	workload, err := fetchWorkload(in.businessLayer, namespace, workloadName)
	if err != nil {
		return nil, err
	}

	var runtimes []kmodel.Runtime
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		conf := config.Get()
		app := workload.Labels[conf.IstioLabels.AppLabelName]
		version := workload.Labels[conf.IstioLabels.VersionLabelName]
		dash := NewDashboardsService(in.prom)
		runtimes = dash.GetCustomDashboardRefs(namespace, app, version, workload.Pods)
	}()

	if includeServices {
		var services []core_v1.Service
		var err error
		// Check if namespace is cached
		if kialiCache != nil && kialiCache.CheckNamespace(namespace) {
			// Cache uses Kiali ServiceAccount, check if user can access to the namespace
			if _, err = in.businessLayer.Namespace.GetNamespace(namespace); err == nil {
				services, err = kialiCache.GetServices(namespace, workload.Labels)
			}
		} else {
			services, err = in.k8s.GetServices(namespace, workload.Labels)
		}
		if err != nil {
			return nil, err
		}
		workload.SetServices(services)
	}

	wg.Wait()
	workload.Runtimes = runtimes

	return workload, nil
}

func (in *WorkloadService) GetPods(namespace string, labelSelector string) (models.Pods, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "WorkloadService", "GetPods")
	defer promtimer.ObserveNow(&err)

	var ps []core_v1.Pod
	// Check if namespace is cached
	if kialiCache != nil && kialiCache.CheckNamespace(namespace) {
		// Cache uses Kiali ServiceAccount, check if user can access to the namespace
		if _, err = in.businessLayer.Namespace.GetNamespace(namespace); err == nil {
			ps, err = kialiCache.GetPods(namespace, labelSelector)
		}
	} else {
		ps, err = in.k8s.GetPods(namespace, labelSelector)
	}

	if err != nil {
		return nil, err
	}
	pods := models.Pods{}
	pods.Parse(ps)
	return pods, nil
}

func (in *WorkloadService) GetPod(namespace, name string) (*models.Pod, error) {
	var err error
	promtimer := internalmetrics.GetGoFunctionMetric("business", "WorkloadService", "GetPod")
	defer promtimer.ObserveNow(&err)

	p, err := in.k8s.GetPod(namespace, name)
	if err != nil {
		return nil, err
	}
	pod := models.Pod{}
	pod.Parse(p)
	return &pod, nil
}

func (in *WorkloadService) GetPodLogs(namespace, name string, opts *core_v1.PodLogOptions) (*kubernetes.PodLogs, error) {
	return in.k8s.GetPodLogs(namespace, name, opts)
}

//fetchWorkloads 这个地方应该优化
func fetchWorkloads(layer *Layer, namespace string, labelSelector string) (models.Workloads, error) {
	var pods []core_v1.Pod
	var repcon []core_v1.ReplicationController
	var dep []apps_v1.Deployment
	var repset []apps_v1.ReplicaSet
	var depcon []osapps_v1.DeploymentConfig
	var fulset []apps_v1.StatefulSet
	var jbs []batch_v1.Job
	var conjbs []batch_v1beta1.CronJob

	ws := models.Workloads{}
	kCache := *GetKialiCache(layer.Host)
	if kCache == nil {
		return nil, errors2.New("kiali cache not found")
	}
	// Check if user has access to the namespace (RBAC) in cache scenarios and/or
	// if namespace is accessible from Kiali (Deployment.AccessibleNamespaces)
	//检查用户是否可以在缓存方案中访问名称空间（RBAC）和
	//是否可以从Kiali（Deployment.AccessibleNamespaces）访问名称空间
	wg := sync.WaitGroup{}
	wg.Add(8)
	errChan := make(chan error, 8)

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper caller
		if kCache != nil && kCache.CheckNamespace(namespace) {
			pods, err = kCache.GetPods(namespace, labelSelector)
		} else {
			pods, err = layer.k8s.GetPods(namespace, labelSelector)
		}
		if err != nil {
			log.Errorf("Error fetching Pods per namespace %s: %s", namespace, err)
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper caller
		if kCache != nil && kCache.CheckNamespace(namespace) {
			dep, err = kCache.GetDeployments(namespace)
		} else {
			dep, err = layer.k8s.GetDeployments(namespace)
		}
		if err != nil {
			log.Errorf("Error fetching Deployments per namespace %s: %s", namespace, err)
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper caller
		if kCache != nil && kCache.CheckNamespace(namespace) {
			repset, err = kCache.GetReplicaSets(namespace)
		} else {
			repset, err = layer.k8s.GetReplicaSets(namespace)
		}
		if err != nil {
			log.Errorf("Error fetching ReplicaSets per namespace %s: %s", namespace, err)
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.ReplicationControllerType) {
			repcon, err = layer.k8s.GetReplicationControllers(namespace)
			if err != nil {
				log.Errorf("Error fetching GetReplicationControllers per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if layer.k8s.IsOpenShift() && isWorkloadIncluded(kubernetes.DeploymentConfigType) {
			depcon, err = layer.k8s.GetDeploymentConfigs(namespace)
			if err != nil {
				log.Errorf("Error fetching DeploymentConfigs per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.StatefulSetType) {
			fulset, err = layer.k8s.GetStatefulSets(namespace)
			if err != nil {
				log.Errorf("Error fetching StatefulSets per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.CronJobType) {
			conjbs, err = layer.k8s.GetCronJobs(namespace)
			if err != nil {
				log.Errorf("Error fetching CronJobs per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.JobType) {
			jbs, err = layer.k8s.GetJobs(namespace)
			if err != nil {
				log.Errorf("Error fetching Jobs per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	wg.Wait()
	if len(errChan) != 0 {
		err := <-errChan
		return ws, err
	}
	// 以上部分是找到所有的控制器以及pod

	// Key: name of controller; Value: type of controller
	controllers := map[string]string{}

	// Find controllers from pods
	// 把pod放到控制器中
	for _, pod := range pods {
		if len(pod.OwnerReferences) != 0 {
			for _, ref := range pod.OwnerReferences {
				if ref.Controller != nil && *ref.Controller {
					if _, exist := controllers[ref.Name]; !exist {
						controllers[ref.Name] = ref.Kind
					} else {
						if controllers[ref.Name] != ref.Kind {
							controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
						}
					}
				}
			}
		} else {
			if _, exist := controllers[pod.Name]; !exist {
				// Pod without controller
				controllers[pod.Name] = "Pod"
			}
		}
	}

	// Resolve ReplicaSets from Deployments
	// Resolve ReplicationControllers from DeploymentConfigs
	// Resolve Jobs from CronJobs
	for cname, ctype := range controllers {
		if ctype == "ReplicaSet" {
			found := false
			iFound := -1
			for i, rs := range repset {
				if rs.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(repset[iFound].OwnerReferences) > 0 {
				for _, ref := range repset[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child ReplicaSet and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						delete(controllers, cname)
					}
				}
			}
		}
		if ctype == "ReplicationController" {
			found := false
			iFound := -1
			for i, rc := range repcon {
				if rc.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(repcon[iFound].OwnerReferences) > 0 {
				for _, ref := range repcon[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child ReplicationController and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						delete(controllers, cname)
					}
				}
			}
		}
		if ctype == "Job" {
			found := false
			iFound := -1
			for i, jb := range jbs {
				if jb.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(jbs[iFound].OwnerReferences) > 0 {
				for _, ref := range jbs[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child Job and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						// Jobs are special as deleting CronJob parent doesn't delete children
						// So we need to check that parent exists before to delete children controller
						cnExist := false
						for _, cnj := range conjbs {
							if cnj.Name == ref.Name {
								cnExist = true
								break
							}
						}
						if cnExist {
							delete(controllers, cname)
						}
					}
				}
			}
		}
	}

	// Cornercase, check for controllers without pods, to show them as a workload
	var selector labels.Selector
	var selErr error
	if labelSelector != "" {
		selector, selErr = labels.Parse(labelSelector)
		if selErr != nil {
			log.Errorf("%s can not be processed as selector: %v", labelSelector, selErr)
		}
	}
	for _, d := range dep {
		selectorCheck := true
		if selector != nil {
			selectorCheck = selector.Matches(labels.Set(d.Spec.Template.Labels))
		}
		if _, exist := controllers[d.Name]; !exist && selectorCheck {
			controllers[d.Name] = "Deployment"
		}
	}
	for _, rs := range repset {
		selectorCheck := true
		if selector != nil {
			selectorCheck = selector.Matches(labels.Set(rs.Spec.Template.Labels))
		}
		if _, exist := controllers[rs.Name]; !exist && len(rs.OwnerReferences) == 0 && selectorCheck {
			controllers[rs.Name] = "ReplicaSet"
		}
	}
	for _, dc := range depcon {
		selectorCheck := true
		if selector != nil {
			selectorCheck = selector.Matches(labels.Set(dc.Spec.Template.Labels))
		}
		if _, exist := controllers[dc.Name]; !exist && selectorCheck {
			controllers[dc.Name] = "DeploymentConfig"
		}
	}
	for _, rc := range repcon {
		selectorCheck := true
		if selector != nil {
			selectorCheck = selector.Matches(labels.Set(rc.Spec.Template.Labels))
		}
		if _, exist := controllers[rc.Name]; !exist && len(rc.OwnerReferences) == 0 && selectorCheck {
			controllers[rc.Name] = "ReplicationController"
		}
	}
	for _, fs := range fulset {
		selectorCheck := true
		if selector != nil {
			selectorCheck = selector.Matches(labels.Set(fs.Spec.Template.Labels))
		}
		if _, exist := controllers[fs.Name]; !exist && selectorCheck {
			controllers[fs.Name] = "StatefulSet"
		}
	}

	// Build workloads from controllers
	var cnames []string
	for k := range controllers {
		cnames = append(cnames, k)
	}
	//cnames 这个玩意是啥
	sort.Strings(cnames)
	for _, cname := range cnames {
		w := &models.Workload{
			Pods:     models.Pods{},
			Services: models.Services{},
		}
		ctype := controllers[cname]
		// Flag to add a controller if it is found
		cnFound := true
		switch ctype {
		case "Deployment":
			found := false
			iFound := -1
			for i, dp := range dep {
				if dp.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(dep[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseDeployment(&dep[iFound])
			} else {
				log.Errorf("Workload %s is not found as Deployment", cname)
				cnFound = false
			}
		case "ReplicaSet":
			found := false
			iFound := -1
			for i, rs := range repset {
				if rs.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(repset[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseReplicaSet(&repset[iFound])
			} else {
				log.Errorf("Workload %s is not found as ReplicaSet", cname)
				cnFound = false
			}
		case "ReplicationController":
			found := false
			iFound := -1
			for i, rc := range repcon {
				if rc.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(repcon[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseReplicationController(&repcon[iFound])
			} else {
				log.Errorf("Workload %s is not found as ReplicationController", cname)
				cnFound = false
			}
		case "DeploymentConfig":
			found := false
			iFound := -1
			for i, dc := range depcon {
				if dc.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(depcon[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseDeploymentConfig(&depcon[iFound])
			} else {
				log.Errorf("Workload %s is not found as DeploymentConfig", cname)
				cnFound = false
			}
		case "StatefulSet":
			found := false
			iFound := -1
			for i, fs := range fulset {
				if fs.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(fulset[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseStatefulSet(&fulset[iFound])
			} else {
				log.Errorf("Workload %s is not found as StatefulSet", cname)
				cnFound = false
			}
		case "Pod":
			found := false
			iFound := -1
			for i, pod := range pods {
				if pod.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				w.SetPods([]core_v1.Pod{pods[iFound]})
				w.ParsePod(&pods[iFound])
			} else {
				log.Errorf("Workload %s is not found as Pod", cname)
				cnFound = false
			}
		case "Job":
			found := false
			iFound := -1
			for i, jb := range jbs {
				if jb.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(jbs[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseJob(&jbs[iFound])
			} else {
				log.Errorf("Workload %s is not found as Job", cname)
				cnFound = false
			}
		case "CronJob":
			found := false
			iFound := -1
			for i, cjb := range conjbs {
				if cjb.Name == cname {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(conjbs[iFound].Spec.JobTemplate.Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseCronJob(&conjbs[iFound])
			} else {
				log.Warningf("Workload %s is not found as CronJob (CronJob could be deleted but children are still in the namespace)", cname)
				cnFound = false
			}
		default:
			cPods := kubernetes.FilterPodsForController(cname, ctype, pods)
			w.SetPods(cPods)
			w.ParsePods(cname, ctype, cPods)
		}
		if cnFound {
			ws = append(ws, w)
		}
	}
	return ws, nil
}

func fetchWorkload(layer *Layer, namespace string, workloadName string) (*models.Workload, error) {
	var pods []core_v1.Pod
	var repcon []core_v1.ReplicationController
	var dep *apps_v1.Deployment
	var repset []apps_v1.ReplicaSet
	var depcon *osapps_v1.DeploymentConfig
	var fulset *apps_v1.StatefulSet
	var jbs []batch_v1.Job
	var conjbs []batch_v1beta1.CronJob

	wl := &models.Workload{
		Pods:     models.Pods{},
		Services: models.Services{},
	}

	// Check if user has access to the namespace (RBAC) in cache scenarios and/or
	// if namespace is accessible from Kiali (Deployment.AccessibleNamespaces)
	if _, err := layer.Namespace.GetNamespace(namespace); err != nil {
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(8)
	errChan := make(chan error, 8)

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper call
		if kialiCache != nil && kialiCache.CheckNamespace(namespace) {
			pods, err = kialiCache.GetPods(namespace, "")
		} else {
			pods, err = layer.k8s.GetPods(namespace, "")
		}
		if err != nil {
			log.Errorf("Error fetching Pods per namespace %s: %s", namespace, err)
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper call
		if kialiCache != nil && kialiCache.CheckNamespace(namespace) {
			dep, err = kialiCache.GetDeployment(namespace, workloadName)
		} else {
			dep, err = layer.k8s.GetDeployment(namespace, workloadName)
		}
		if err != nil {
			if errors.IsNotFound(err) {
				dep = nil
			} else {
				log.Errorf("Error fetching Deployment per namespace %s and name %s: %s", namespace, workloadName, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		// Check if namespace is cached
		// Namespace access is checked in the upper call
		if kialiCache != nil && kialiCache.CheckNamespace(namespace) {
			repset, err = kialiCache.GetReplicaSets(namespace)
		} else {
			repset, err = layer.k8s.GetReplicaSets(namespace)
		}
		if err != nil {
			log.Errorf("Error fetching ReplicaSets per namespace %s: %s", namespace, err)
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.ReplicationControllerType) {
			repcon, err = layer.k8s.GetReplicationControllers(namespace)
			if err != nil {
				log.Errorf("Error fetching GetReplicationControllers per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if layer.k8s.IsOpenShift() && isWorkloadIncluded(kubernetes.DeploymentConfigType) {
			depcon, err = layer.k8s.GetDeploymentConfig(namespace, workloadName)
			if err != nil {
				depcon = nil
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.StatefulSetType) {
			fulset, err = layer.k8s.GetStatefulSet(namespace, workloadName)
			if err != nil {
				fulset = nil
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.CronJobType) {
			conjbs, err = layer.k8s.GetCronJobs(namespace)
			if err != nil {
				log.Errorf("Error fetching CronJobs per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		if isWorkloadIncluded(kubernetes.JobType) {
			jbs, err = layer.k8s.GetJobs(namespace)
			if err != nil {
				log.Errorf("Error fetching Jobs per namespace %s: %s", namespace, err)
				errChan <- err
			}
		}
	}()

	wg.Wait()
	if len(errChan) != 0 {
		err := <-errChan
		return wl, err
	}

	// Key: name of controller; Value: type of controller
	controllers := map[string]string{}

	// Find controllers from pods
	for _, pod := range pods {
		if len(pod.OwnerReferences) != 0 {
			for _, ref := range pod.OwnerReferences {
				if ref.Controller != nil && *ref.Controller {
					if _, exist := controllers[ref.Name]; !exist {
						controllers[ref.Name] = ref.Kind
					} else {
						if controllers[ref.Name] != ref.Kind {
							controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
						}
					}
				}
			}
		} else {
			if _, exist := controllers[pod.Name]; !exist {
				// Pod without controller
				controllers[pod.Name] = "Pod"
			}
		}
	}

	// Resolve ReplicaSets from Deployments
	// Resolve ReplicationControllers from DeploymentConfigs
	// Resolve Jobs from CronJobs
	for cname, ctype := range controllers {
		if ctype == "ReplicaSet" {
			found := false
			iFound := -1
			for i, rs := range repset {
				if rs.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(repset[iFound].OwnerReferences) > 0 {
				for _, ref := range repset[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child ReplicaSet and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						delete(controllers, cname)
					}
				}
			}
		}
		if ctype == "ReplicationController" {
			found := false
			iFound := -1
			for i, rc := range repcon {
				if rc.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(repcon[iFound].OwnerReferences) > 0 {
				for _, ref := range repcon[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child ReplicationController and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						delete(controllers, cname)
					}
				}
			}
		}
		if ctype == "Job" {
			found := false
			iFound := -1
			for i, jb := range jbs {
				if jb.Name == cname {
					iFound = i
					found = true
					break
				}
			}
			if found && len(jbs[iFound].OwnerReferences) > 0 {
				for _, ref := range jbs[iFound].OwnerReferences {
					if ref.Controller != nil && *ref.Controller {
						// Delete the child Job and add the parent controller
						if _, exist := controllers[ref.Name]; !exist {
							controllers[ref.Name] = ref.Kind
						} else {
							if controllers[ref.Name] != ref.Kind {
								controllers[ref.Name] = controllerPriority(controllers[ref.Name], ref.Kind)
							}
						}
						// Jobs are special as deleting CronJob parent doesn't delete children
						// So we need to check that parent exists before to delete children controller
						cnExist := false
						for _, cnj := range conjbs {
							if cnj.Name == ref.Name {
								cnExist = true
								break
							}
						}
						if cnExist {
							delete(controllers, cname)
						}
					}
				}
			}
		}
	}

	// Cornercase, check for controllers without pods, to show them as a workload
	if dep != nil {
		if _, exist := controllers[dep.Name]; !exist {
			controllers[dep.Name] = "Deployment"
		}
	}
	for _, rs := range repset {
		if _, exist := controllers[rs.Name]; !exist && len(rs.OwnerReferences) == 0 {
			controllers[rs.Name] = "ReplicaSet"
		}
	}
	if depcon != nil {
		if _, exist := controllers[depcon.Name]; !exist {
			controllers[depcon.Name] = "DeploymentConfig"
		}
	}
	for _, rc := range repcon {
		if _, exist := controllers[rc.Name]; !exist && len(rc.OwnerReferences) == 0 {
			controllers[rc.Name] = "ReplicationController"
		}
	}
	if fulset != nil {
		if _, exist := controllers[fulset.Name]; !exist {
			controllers[fulset.Name] = "StatefulSet"
		}
	}

	// Build workload from controllers

	if _, exist := controllers[workloadName]; exist {
		w := models.Workload{
			Pods:     models.Pods{},
			Services: models.Services{},
		}
		ctype := controllers[workloadName]
		// Flag to add a controller if it is found
		cnFound := true
		switch ctype {
		case "Deployment":
			if dep.Name == workloadName {
				selector := labels.Set(dep.Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseDeployment(dep)
			} else {
				log.Errorf("Workload %s is not found as Deployment", workloadName)
				cnFound = false
			}
		case "ReplicaSet":
			found := false
			iFound := -1
			for i, rs := range repset {
				if rs.Name == workloadName {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(repset[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseReplicaSet(&repset[iFound])
			} else {
				log.Errorf("Workload %s is not found as ReplicaSet", workloadName)
				cnFound = false
			}
		case "ReplicationController":
			found := false
			iFound := -1
			for i, rc := range repcon {
				if rc.Name == workloadName {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(repcon[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseReplicationController(&repcon[iFound])
			} else {
				log.Errorf("Workload %s is not found as ReplicationController", workloadName)
				cnFound = false
			}
		case "DeploymentConfig":
			if depcon.Name == workloadName {
				selector := labels.Set(depcon.Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseDeploymentConfig(depcon)
			} else {
				log.Errorf("Workload %s is not found as DeploymentConfig", workloadName)
				cnFound = false
			}
		case "StatefulSet":
			if fulset.Name == workloadName {
				selector := labels.Set(fulset.Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseStatefulSet(fulset)
			} else {
				log.Errorf("Workload %s is not found as StatefulSet", workloadName)
				cnFound = false
			}
		case "Pod":
			found := false
			iFound := -1
			for i, pod := range pods {
				if pod.Name == workloadName {
					found = true
					iFound = i
					break
				}
			}
			if found {
				w.SetPods([]core_v1.Pod{pods[iFound]})
				w.ParsePod(&pods[iFound])
			} else {
				log.Errorf("Workload %s is not found as Pod", workloadName)
				cnFound = false
			}
		case "Job":
			found := false
			iFound := -1
			for i, jb := range jbs {
				if jb.Name == workloadName {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(jbs[iFound].Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseJob(&jbs[iFound])
			} else {
				log.Errorf("Workload %s is not found as Job", workloadName)
				cnFound = false
			}
		case "CronJob":
			found := false
			iFound := -1
			for i, cjb := range conjbs {
				if cjb.Name == workloadName {
					found = true
					iFound = i
					break
				}
			}
			if found {
				selector := labels.Set(conjbs[iFound].Spec.JobTemplate.Spec.Template.Labels).AsSelector()
				w.SetPods(kubernetes.FilterPodsForSelector(selector, pods))
				w.ParseCronJob(&conjbs[iFound])
			} else {
				log.Warningf("Workload %s is not found as CronJob (CronJob could be deleted but children are still in the namespace)", workloadName)
				cnFound = false
			}
		default:
			cPods := kubernetes.FilterPodsForController(workloadName, ctype, pods)
			w.SetPods(cPods)
			w.ParsePods(workloadName, ctype, cPods)
		}
		if cnFound {
			return &w, nil
		}
	}
	return wl, kubernetes.NewNotFound(workloadName, "Kiali", "Workload")
}

// KIALI-1730
// This method is used to decide the priority of the controller in the cornercase when two controllers have same labels
// on the selector. Note that this is a situation that user should control as it is described in the documentation:
// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
// But Istio only identifies one controller as workload (it doesn't note which one).
// Kiali can select one on the list of workloads and other in the details and this should be consistent.
var controllerOrder = map[string]int{
	"Deployment":            6,
	"DeploymentConfig":      5,
	"ReplicaSet":            4,
	"ReplicationController": 3,
	"StatefulSet":           2,
	"Job":                   1,
	"DaemonSet":             0,
	"Pod":                   -1,
}

func controllerPriority(type1, type2 string) string {
	w1, e1 := controllerOrder[type1]
	if !e1 {
		log.Errorf("This controller %s is assigned in a Pod and it's not properly managed", type1)
	}
	w2, e2 := controllerOrder[type2]
	if !e2 {
		log.Errorf("This controller %s is assigned in a Pod and it's not properly managed", type2)
	}
	if w1 >= w2 {
		return type1
	} else {
		return type2
	}
}
