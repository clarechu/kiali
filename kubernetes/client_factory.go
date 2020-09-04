package kubernetes

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sync"
	"time"

	"github.com/kiali/kiali/log"
	"github.com/kiali/kiali/prometheus/internalmetrics"
)

var factory *clientFactory

// Mutex for when modifying the stored clients
var mutex = &sync.RWMutex{}

const expirationTime = time.Minute * 15

// ClientFactory interface for the clientFactory object
type ClientFactory interface {
	GetClient(token string) (IstioClientInterface, error)
	GetClientNoAuth() (IstioClientInterface, error)
}

// clientFactory used to generate per users clients
type clientFactory struct {
	ClientFactory
	baseIstioConfig *rest.Config
	clientEntries   map[string]*clientEntry
}

// clientEntry stored the client and its created timestamp
type clientEntry struct {
	client  IstioClientInterface
	created time.Time
}

// GetClientFactory returns the client factory. Creates a new one if necessary
func GetClientFactory() (ClientFactory, error) {
	if factory == nil {
		// Get the normal configuration
		config, err := ConfigClient()
		if err != nil {
			return nil, err
		}

		// Create a new config based on what was gathered above but don't specify the bearer token to use
		istioConfig := rest.Config{
			Host:            config.Host,
			TLSClientConfig: config.TLSClientConfig,
			QPS:             config.QPS,
			Burst:           config.Burst,
		}

		return getClientFactory(&istioConfig, expirationTime)

	}
	return factory, nil
}

// GetClientFactory returns the client factory. Creates a new one if necessary
func GetClientFileFactory(config *rest.Config) (ClientFactory, error) {
	// Create a new config based on what was gathered above but don't specify the bearer token to use
	return getClientFactory(config, expirationTime)
}

func GetK8sClientSet(config *rest.Config) (clientSet *kubernetes.Clientset, err error) {
	clientSet, err = kubernetes.NewForConfig(config)
	return
}

// LoadFromFile takes a filename and deserializes the contents into Config object
func LoadFromFile(kubeconfigBytes []byte) (*clientcmdapi.Config, error) {
	config, err := clientcmd.Load(kubeconfigBytes)
	if err != nil {
		return nil, err
	}

	// set LocationOfOrigin on every Cluster, User, and Context
	for key, obj := range config.AuthInfos {
		obj.LocationOfOrigin = "default"
		config.AuthInfos[key] = obj
	}
	for key, obj := range config.Clusters {
		obj.LocationOfOrigin = "default"
		config.Clusters[key] = obj
	}
	for key, obj := range config.Contexts {
		obj.LocationOfOrigin = "default"
		config.Contexts[key] = obj
	}

	if config.AuthInfos == nil {
		config.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	}
	if config.Clusters == nil {
		config.Clusters = map[string]*clientcmdapi.Cluster{}
	}
	if config.Contexts == nil {
		config.Contexts = map[string]*clientcmdapi.Context{}
	}

	return config, nil
}

// newClientFactory allows for specifying the config and expiry duration
// Mock friendly for testing purposes
func getClientFactory(istioConfig *rest.Config, expiry time.Duration) (*clientFactory, error) {
	mutex.Lock()
	if factory == nil {
		clientEntriesMap := make(map[string]*clientEntry)

		factory = &clientFactory{
			baseIstioConfig: istioConfig,
			clientEntries:   clientEntriesMap,
		}

		go watchClients(clientEntriesMap, expiry)
	}
	mutex.Unlock()
	return factory, nil
}

// NewClient creates a new IstioClientInterface based on a users k8s token
func (cf *clientFactory) newClient(token string) (IstioClientInterface, error) {
	config := cf.baseIstioConfig

	config.BearerToken = token

	return NewClientFromConfig(config)
}

// NewClient creates a new IstioClientInterface based on a users k8s token
func (cf *clientFactory) newClientNoAuth() (IstioClientInterface, error) {
	config := cf.baseIstioConfig
	return NewClientFromConfig(config)
}

// GetClient returns a client for the specified token. Creating one if necessary.
func (cf *clientFactory) GetClient(token string) (IstioClientInterface, error) {
	clientEntry, err := cf.getClientEntry(token)
	if err != nil {
		return nil, err
	}
	return clientEntry.client, nil
}

// GetClient returns a client for the specified token. Creating one if necessary.
func (cf *clientFactory) GetClientNoAuth() (IstioClientInterface, error) {
	clientEntry, err := cf.getClientEntryNoAuth()
	if err != nil {
		return nil, err
	}
	return clientEntry.client, nil
}

// getClientEntry returns a clientEntry for the specified token. Creating one if necessary.
func (cf *clientFactory) getClientEntry(token string) (*clientEntry, error) {
	mutex.RLock()
	cEntry, ok := cf.clientEntries[token]
	mutex.RUnlock()
	if ok {
		return cEntry, nil
	} else {
		client, err := cf.newClient(token)
		if err != nil {
			log.Errorf("Error fetching the Kubernetes client: %v", err)
			return nil, err
		}

		cEntry := clientEntry{
			client:  client,
			created: time.Now(),
		}

		mutex.Lock()
		cf.clientEntries[token] = &cEntry
		mutex.Unlock()
		internalmetrics.SetKubernetesClients(len(cf.clientEntries))
		return &cEntry, nil
	}
}

// getClientEntry returns a clientEntry for the specified token. Creating one if necessary.
func (cf *clientFactory) getClientEntryNoAuth() (*clientEntry, error) {
	client, err := cf.newClientNoAuth()
	if err != nil {
		log.Errorf("Error fetching the Kubernetes client: %v", err)
		return nil, err
	}
	cEntry := clientEntry{
		client:  client,
		created: time.Now(),
	}

	mutex.Lock()
	mutex.Unlock()
	internalmetrics.SetKubernetesClients(len(cf.clientEntries))
	return &cEntry, nil
}

// watchClients loops over clients and removes ones which are too old
func watchClients(clientEntries map[string]*clientEntry, expiry time.Duration) {
	for {
		time.Sleep(expiry)
		mutex.Lock()
		for token, clientEntry := range clientEntries {
			if time.Since(clientEntry.created) > expiry {
				delete(clientEntries, token)
			}
		}
		internalmetrics.SetKubernetesClients(len(clientEntries))
		mutex.Unlock()
	}
}
