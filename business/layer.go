package business

import (
	"github.com/kiali/kiali/config"
	"github.com/kiali/kiali/jaeger"
	"github.com/kiali/kiali/kubernetes"
	"github.com/kiali/kiali/kubernetes/cache"
	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
)

// Layer is a container for fast access to inner services
type Layer struct {
	Svc            SvcService
	Health         HealthService
	Validations    IstioValidationsService
	IstioConfig    IstioConfigService
	Workload       WorkloadService
	App            AppService
	Namespace      NamespaceService
	Jaeger         JaegerService
	k8s            kubernetes.IstioClientInterface
	OpenshiftOAuth OpenshiftOAuthService
	TLS            TLSService
	ThreeScale     ThreeScaleService
	Iter8          Iter8Service
	IstioStatus    IstioStatusService
	Context        string
}

// Global clientfactory and prometheus clients.
var clientFactory kubernetes.ClientFactory
var prometheusClient prometheus.ClientInterface
var once sync.Once
var kialiCache cache.KialiCache

func initKialiCache() {
	if config.Get().KubernetesConfig.CacheEnabled {
		if cache, err := cache.NewKialiCache(); err != nil {
			log.Errorf("Error initializing Kiali Cache. Details: %s", err)
		} else {
			kialiCache = cache
		}
	}
	if excludedWorkloads == nil {
		excludedWorkloads = make(map[string]bool)
		for _, w := range config.Get().KubernetesConfig.ExcludeWorkloads {
			excludedWorkloads[w] = true
		}
	}
}

func GetUnauthenticated() (*Layer, error) {
	return Get("")
}

// Get the business.Layer
func Get(token string) (*Layer, error) {
	// Kiali Cache will be initialized once at first use of Business layer
	once.Do(initKialiCache)

	// Use an existing client factory if it exists, otherwise create and use in the future
	if clientFactory == nil {
		userClient, err := kubernetes.GetClientFactory()
		if err != nil {
			return nil, err
		}
		clientFactory = userClient
	}

	// Creates a new k8s client based on the current users token
	k8s, err := clientFactory.GetClient(token)
	if err != nil {
		return nil, err
	}

	// Use an existing Prometheus client if it exists, otherwise create and use in the future
	if prometheusClient == nil {
		prom, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		prometheusClient = prom
	}

	// Create Jaeger client
	jaegerLoader := func() (jaeger.ClientInterface, error) {
		return jaeger.NewClient(token)
	}

	return NewWithBackends(k8s, prometheusClient, jaegerLoader), nil
}

const (
	DefaultNamespace = "service-mesh"
	KubeConfig       = "kubeConfig"
)

func GetConfigMap(name string) (configMap *v1.ConfigMap, err error) {
	ops := metav1.GetOptions{}
	clientSet, err := kubernetes.GetDefaultK8sClientSet()
	if err != nil {
		return nil, err
	}
	return clientSet.CoreV1().ConfigMaps(DefaultNamespace).Get(name, ops)
}

// Get the business.Layer
func GetNoAuth(name string) (*Layer, error) {
	// Kiali Cache will be initialized once at first use of Business layer
	once.Do(initKialiCache)
	configMap, err := GetConfigMap(name)
	if err != nil {
		return nil, err
	}
	// Use an existing client factory if it exists, otherwise create and use in the future
	if clientFactory == nil {
		userClient, err := kubernetes.GetClientFileFactory(configMap.BinaryData[KubeConfig])
		if err != nil {
			return nil, err
		}
		clientFactory = userClient
	}

	// Creates a new k8s client based on the current users token
	k8s, err := clientFactory.GetClientNoAuth()
	if err != nil {
		return nil, err
	}

	// Use an existing Prometheus client if it exists, otherwise create and use in the future
	if prometheusClient == nil {
		prom, err := prometheus.NewClientNoAuth(name)
		if err != nil {
			return nil, err
		}
		prometheusClient = prom
	}

	// Create Jaeger client
	jaegerLoader := func() (jaeger.ClientInterface, error) {
		return jaeger.NewClientNoAuth()
	}
	layer := NewWithBackends(k8s, prometheusClient, jaegerLoader)
	layer.Context = name
	return layer, nil
}

// SetWithBackends allows for specifying the ClientFactory and Prometheus clients to be used.
// Mock friendly. Used only with tests.
func SetWithBackends(cf kubernetes.ClientFactory, prom prometheus.ClientInterface) {
	clientFactory = cf
	prometheusClient = prom
}

// NewWithBackends creates the business layer using the passed k8s and prom clients
func NewWithBackends(k8s kubernetes.IstioClientInterface, prom prometheus.ClientInterface, jaegerClient JaegerLoader) *Layer {
	temporaryLayer := &Layer{}
	temporaryLayer.Health = HealthService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Svc = SvcService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.IstioConfig = IstioConfigService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Workload = WorkloadService{k8s: k8s, prom: prom, businessLayer: temporaryLayer}
	temporaryLayer.Validations = IstioValidationsService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.App = AppService{prom: prom, k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.Namespace = NewNamespaceService(k8s)
	temporaryLayer.Jaeger = JaegerService{loader: jaegerClient, businessLayer: temporaryLayer}
	temporaryLayer.k8s = k8s
	temporaryLayer.OpenshiftOAuth = OpenshiftOAuthService{k8s: k8s}
	temporaryLayer.TLS = TLSService{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.ThreeScale = ThreeScaleService{k8s: k8s}
	temporaryLayer.Iter8 = Iter8Service{k8s: k8s, businessLayer: temporaryLayer}
	temporaryLayer.IstioStatus = IstioStatusService{k8s: k8s}

	return temporaryLayer
}

func Stop() {
	if kialiCache != nil {
		kialiCache.Stop()
	}
}
