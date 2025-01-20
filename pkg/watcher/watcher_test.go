package watcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeClient "k8s.io/client-go/kubernetes/fake"
)

func TestGetPodsByIP(t *testing.T) {
	t.Parallel()

	clientSet := fakeClient.NewSimpleClientset()
	podClient := clientSet.CoreV1().Pods("namespace")
	watcher, err := NewWatcher("", clientSet)
	assert.NoError(t, err)

	_, err = podClient.Create(context.Background(), &corev1.Pod{Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "1.2.3.4"}}}, ObjectMeta: metav1.ObjectMeta{Name: "keepme1"}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = podClient.Create(context.Background(), &corev1.Pod{Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "1.2.3.4"}}}, ObjectMeta: metav1.ObjectMeta{Name: "keepme2"}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = podClient.Create(context.Background(), &corev1.Pod{Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "4.3.2.1"}}}, ObjectMeta: metav1.ObjectMeta{Name: "keepme3"}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = podClient.Create(context.Background(), &corev1.Pod{Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "4.3.2.1"}}}, ObjectMeta: metav1.ObjectMeta{Name: "deleteme"}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	err = podClient.Delete(context.Background(), "deleteme", metav1.DeleteOptions{})
	assert.NoError(t, err)

	// Wait for the informers to catch up.
	time.Sleep(5 * time.Second)

	pods, err := watcher.GetPodsByIP("1.2.3.4")
	assert.NoError(t, err)
	assert.Len(t, pods, 2)
	names := []string{}
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	assert.Contains(t, names, "keepme1")
	assert.Contains(t, names, "keepme2")

	pods, err = watcher.GetPodsByIP("4.3.2.1")
	assert.NoError(t, err)
	assert.Len(t, pods, 1)
	assert.Equal(t, "keepme3", pods[0].Name)

	pods, err = watcher.GetPodsByIP("0.0.0.0")
	assert.NoError(t, err)
	assert.Len(t, pods, 0)
}

func TestGetNodeByInternalIP(t *testing.T) {
	t.Parallel()

	clientSet := fakeClient.NewSimpleClientset()
	nodeClient := clientSet.CoreV1().Nodes()
	watcher, err := NewWatcher("", clientSet)
	assert.NoError(t, err)

	_, err = nodeClient.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "1.2.3.4"}}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = nodeClient.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "1.2.3.4"}}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = nodeClient.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "4.3.2.1"}}}}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Wait for the informers to catch up.
	time.Sleep(5 * time.Second)

	node, err := watcher.GetNodeByInternalIP("4.3.2.1")
	assert.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "node3", node.Name)

	node, err = watcher.GetNodeByInternalIP("1.2.3.4")
	assert.ErrorIs(t, errNodesShareInternalIP, err)
	assert.Nil(t, node)

	err = nodeClient.Delete(context.Background(), "node1", metav1.DeleteOptions{})
	assert.NoError(t, err)

	time.Sleep(5 * time.Second)
	node, err = watcher.GetNodeByInternalIP("1.2.3.4")
	assert.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "node2", node.Name)
}

func TestGetNodeByName(t *testing.T) {
	t.Parallel()

	clientSet := fakeClient.NewSimpleClientset()
	nodeClient := clientSet.CoreV1().Nodes()
	watcher, err := NewWatcher("", clientSet)
	assert.NoError(t, err)

	_, err = nodeClient.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "1.2.3.4"}}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = nodeClient.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "1.2.3.4"}}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	err = nodeClient.Delete(context.Background(), "node1", metav1.DeleteOptions{})
	assert.NoError(t, err)

	// Wait for the informers to catch up.
	time.Sleep(5 * time.Second)

	node, err := watcher.GetNodeByName("node1")
	assert.NoError(t, err)
	assert.Nil(t, node)

	node, err = watcher.GetNodeByName("node2")
	assert.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "node2", node.Name)
}

func TestGetServiceByClusterIP(t *testing.T) {
	t.Parallel()

	clientSet := fakeClient.NewSimpleClientset()
	serviceClient := clientSet.CoreV1().Services("namespace")
	watcher, err := NewWatcher("", clientSet)
	assert.NoError(t, err)

	_, err = serviceClient.Create(context.Background(), &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service1"}, Spec: corev1.ServiceSpec{ClusterIPs: []string{"1.2.3.4"}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = serviceClient.Create(context.Background(), &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service2"}, Spec: corev1.ServiceSpec{ClusterIPs: []string{"1.2.3.4"}}}, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = serviceClient.Create(context.Background(), &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service3"}, Spec: corev1.ServiceSpec{ClusterIPs: []string{"4.3.2.1"}}}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Wait for the informers to catch up.
	time.Sleep(5 * time.Second)

	service, err := watcher.GetServiceByClusterIP("4.3.2.1")
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "service3", service.Name)

	service, err = watcher.GetServiceByClusterIP("1.2.3.4")
	assert.ErrorIs(t, errServicesShareIP, err)
	assert.Nil(t, service)

	err = serviceClient.Delete(context.Background(), "service1", metav1.DeleteOptions{})
	assert.NoError(t, err)

	time.Sleep(5 * time.Second)
	service, err = watcher.GetServiceByClusterIP("1.2.3.4")
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "service2", service.Name)
}
