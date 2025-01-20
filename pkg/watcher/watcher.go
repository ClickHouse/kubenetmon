package watcher

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Names of indexers for different informers.
const (
	indexByIP         = "indexByIP"
	indexByInternalIP = "indexByInternalIP"
	indexByClusterIP  = "indexByClusterIP"
)

// Errors.
var (
	errNodesShareInternalIP = errors.New("found multiple nodes sharing the same internal IP")
	errServicesShareIP      = errors.New("found multiple services sharing the same IP")
	errNoInformer           = errors.New("informer hasn't been initialised during Watcher creation, reconfigure watchPods, watchNodes, watchServices")
)

// WatcherInterface watches Kubernetes cluster for update.
type WatcherInterface interface {
	GetPodsByIP(ip string) ([]*corev1.Pod, error)
	GetNodeByInternalIP(ip string) (*corev1.Node, error)
	GetNodeByName(name string) (*corev1.Node, error)
	GetServiceByClusterIP(ip string) (*corev1.Service, error)
}

// Watcher implements WatcherInterface.
type Watcher struct {
	// podInformer keeps track of pod identities.
	podInformer cache.SharedIndexInformer
	// nodeInformer keeps track of node identities.
	nodeInformer cache.SharedIndexInformer
	// serviceInformer keeps track of service identities.
	serviceInformer cache.SharedIndexInformer
	// Channel we can send of to tell the informer factory to stop following k8s
	// events.
	informerFactoryStopChannel chan struct{}
}

// NewWatcher creates a new Watcher and checks that it has
// connectivity to the cluster API.
func NewWatcher(clusterName string, clientset kubernetes.Interface) (*Watcher, error) {
	// Create an informer factory.
	factoryStopChannel := make(chan struct{})
	factory := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	// Retrieve pod and node informers.
	var (
		podInformer     cache.SharedIndexInformer
		nodeInformer    cache.SharedIndexInformer
		serviceInformer cache.SharedIndexInformer
	)

	// Configure pod and node informers *before* starting the factory.
	// Index pods by their IPs.
	podInformer = factory.Core().V1().Pods().Informer()
	if err := podInformer.AddIndexers(map[string]cache.IndexFunc{
		indexByIP: func(obj interface{}) ([]string, error) {
			var ips []string
			for _, ip := range obj.(*corev1.Pod).Status.PodIPs {
				ips = append(ips, ip.IP)
			}
			return ips, nil
		},
	}); err != nil {
		return nil, err
	}

	// Index nodes by their internal IPs.
	nodeInformer = factory.Core().V1().Nodes().Informer()
	if err := nodeInformer.AddIndexers(map[string]cache.IndexFunc{
		indexByInternalIP: func(obj interface{}) ([]string, error) {
			var ips []string
			for _, ip := range obj.(*corev1.Node).Status.Addresses {
				if ip.Type == corev1.NodeInternalIP {
					ips = append(ips, ip.Address)
				}
			}
			return ips, nil
		},
	}); err != nil {
		return nil, err
	}

	// Index services by their IPs.
	serviceInformer = factory.Core().V1().Services().Informer()
	if err := serviceInformer.AddIndexers(map[string]cache.IndexFunc{
		indexByClusterIP: func(obj interface{}) ([]string, error) {
			return obj.(*corev1.Service).Spec.ClusterIPs, nil
		},
	}); err != nil {
		return nil, err
	}

	// Start the factory and wait for it to initialise.
	factory.Start(factoryStopChannel)
	factory.WaitForCacheSync(factoryStopChannel)
	log.Info().Msgf("There are currently %d pods, %d nodes, and %d services in the %v cluster!",
		len(podInformer.GetStore().List()),
		len(nodeInformer.GetStore().List()),
		len(serviceInformer.GetStore().List()),
		clusterName,
	)

	return &Watcher{
		podInformer:                podInformer,
		nodeInformer:               nodeInformer,
		serviceInformer:            serviceInformer,
		informerFactoryStopChannel: factoryStopChannel,
	}, nil
}

// GetPodsByIP takes an IP and finds pods on that IP. err is nil if there are no such pods.
func (watcher *Watcher) GetPodsByIP(ip string) ([]*corev1.Pod, error) {
	if watcher.podInformer == nil {
		return nil, errNoInformer
	}

	items, err := watcher.podInformer.GetIndexer().ByIndex(indexByIP, ip)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve pod: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	pods := make([]*corev1.Pod, len(items))
	for i, podItem := range items {
		pods[i] = podItem.(*corev1.Pod)
	}

	return pods, nil
}

// GetNodeByInternalIP takes an IP and finds the node which is represented with
// the IP. err is nil if there is no such node.
func (watcher *Watcher) GetNodeByInternalIP(ip string) (*corev1.Node, error) {
	if watcher.nodeInformer == nil {
		return nil, errNoInformer
	}

	items, err := watcher.nodeInformer.GetIndexer().ByIndex(indexByInternalIP, ip)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve node: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	if len(items) > 1 {
		return nil, errNodesShareInternalIP
	}

	return items[0].(*corev1.Node), nil
}

// GetNodeByName takes a name and finds the node with that name. err is nil if
// there is no such node.
func (watcher *Watcher) GetNodeByName(name string) (*corev1.Node, error) {
	if watcher.nodeInformer == nil {
		return nil, errNoInformer
	}

	item, exists, err := watcher.nodeInformer.GetIndexer().GetByKey(name)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve node: %w", err)
	}

	if !exists {
		return nil, nil
	}

	return item.(*corev1.Node), nil
}

// GetServiceByClusterIP takes an IP and finds the service on the IP. err is nil
// if there is no such service.
func (watcher *Watcher) GetServiceByClusterIP(ip string) (*corev1.Service, error) {
	if watcher.serviceInformer == nil {
		return nil, errNoInformer
	}

	items, err := watcher.serviceInformer.GetIndexer().ByIndex(indexByClusterIP, ip)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve service: %w", err)
	}

	if len(items) == 0 {
		return nil, nil
	}

	if len(items) > 1 {
		return nil, errServicesShareIP
	}

	return items[0].(*corev1.Service), nil
}
