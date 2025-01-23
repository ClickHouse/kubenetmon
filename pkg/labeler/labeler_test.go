package labeler

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/ClickHouse/kubenetmon/pkg/grpc"
	"github.com/ClickHouse/kubenetmon/pkg/watcher"
	mock_watcher "github.com/ClickHouse/kubenetmon/pkg/watcher/mock"
	"github.com/seancfoley/ipaddress-go/ipaddr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errFake = errors.New("fake error")

func TestNewLabeler(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, NewLabeler(nil, nil, false))
}

func TestIsNodeFlow(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	noopRemoteLabeler := &RemoteLabeler{
		remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
	}

	t.Run("Should identify original flows to a node", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP("1.2.3.4").Return(&corev1.Node{}, nil)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		isNode, err := labeler.isNodeFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.True(t, isNode)
	})

	t.Run("Should identify reply flows from a node", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP("1.2.3.4").Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP("4.3.2.1").Return(&corev1.Node{}, nil)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		isNode, err := labeler.isNodeFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.True(t, isNode)
	})

	t.Run("Should return an error when Watcher is erroring out", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP("1.2.3.4").Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP("4.3.2.1").Return(&corev1.Node{}, errFake)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		isNode, err := labeler.isNodeFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
			},
		})
		assert.Error(t, err)
		assert.False(t, isNode)
	})

	t.Run("Should identify flows to services on host network", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP("1.2.3.4").Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP("4.3.2.1").Return(&corev1.Node{}, nil)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		isNode, err := labeler.isNodeFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("0.0.0.0").To4()),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.True(t, isNode)
	})

	t.Run("Should return false for other flows", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP("1.2.3.4").Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP("0.0.0.0").Return(nil, nil)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		isNode, err := labeler.isNodeFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("4.3.2.1").To4()),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("0.0.0.0").To4()),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: binary.BigEndian.Uint32(net.ParseIP("1.2.3.4").To4()),
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.False(t, isNode)
	})
}

func TestLabelFlow(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	t.Run("Should return ErrNodeFlow for a node flow", func(t *testing.T) {
		t.Parallel()

		origSrcIP := netip.MustParseAddr("1.2.3.4")
		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(&corev1.Node{}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow("", &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: 0,
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: 0,
					},
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: 0,
					},
				},
			},
		})

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNodeFlow)
		assert.Nil(t, data)
	})

	t.Run("Should return ErrIgnoredUDPFlow for a UDP flow if configured to do so", func(t *testing.T) {
		t.Parallel()

		origSrcIP := netip.MustParseAddr("1.1.1.1")
		mockWatcher := mock_watcher.NewWatcher(ctrl)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, true)
		data, err := labeler.LabelFlow("", &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V6{
						V6: origSrcIP.AsSlice(),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{},
		})

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrIgnoredUDPFlow)
		assert.Nil(t, data)
	})

	t.Run("Should return ErrIPv6Flow for an IPv6 flow", func(t *testing.T) {
		t.Parallel()

		origSrcIP := netip.MustParseAddr("fe80::dead:beef:70:1")
		mockWatcher := mock_watcher.NewWatcher(ctrl)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow("", &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V6{
						V6: origSrcIP.AsSlice(),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{},
		})

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrIPv6Flow)
		assert.Nil(t, data)
	})

	t.Run("Should correctly label UDP flow from a pod on the node to a public destination in AWS and GCP", func(t *testing.T) {
		t.Parallel()

		// In AWS and GCP, when connecting from a pod to an external
		// destination, the original tuple is (podIP, remoteIP) and the reply
		// tuple (remoteIP, nodeIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			origDstIP      = netip.MustParseAddr("1.2.3.4")
			replySrcIP     = origDstIP
			replyDstIP     = netip.MustParseAddr("10.0.0.2")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           "",
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       PublicInternet,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label UDP flow from a pod on the node to a public destination in Azure", func(t *testing.T) {
		t.Parallel()

		// In Azure, when connecting from a pod to an external destination, the
		// original tuple is (podIP, remoteIP) and the reply tuple is also
		// (remoteIP, podIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			origDstIP      = netip.MustParseAddr("1.2.3.4")
			replySrcIP     = origDstIP
			replyDstIP     = origSrcIP

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                Azure,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           "",
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       PublicInternet,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label TCP flow to a pod on the node from another pod", func(t *testing.T) {
		t.Parallel()

		var (
			localInstance          = "local_instance"
			remoteInstance         = "remote_instance"
			localNamespace         = "local_namespace"
			remoteNamespace        = "remote_namespace"
			localPod               = "local_pod"
			remotePod              = "remote_pod"
			localNode              = "local_node"
			remoteNode             = "remote_node"
			localAvailabilityZone  = "local_availability_zone"
			remoteAvailabilityZone = "remote_availability_zone"
			localApp               = "local_app"
			remoteApp              = "remote_app"
			origSrcIP              = netip.MustParseAddr("10.0.0.1")
			origDstIP              = netip.MustParseAddr("10.0.0.2")
			replySrcIP             = origDstIP
			replyDstIP             = origSrcIP

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		srcPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remotePod,
				Namespace: remoteNamespace,
				Labels: map[string]string{
					"control-plane-id": remoteInstance,
					"k8s-app":          remoteApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: remoteNode,
			},
		}

		dstPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      localPod,
				Namespace: localNamespace,
				Labels: map[string]string{
					"control-plane-id":       localInstance,
					"app.kubernetes.io/name": localApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: localNode,
			},
		}

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{srcPod}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return([]*corev1.Pod{dstPod}, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": localAvailabilityZone,
			}}}, nil)
		mockWatcher.EXPECT().GetNodeByName(remoteNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": remoteAvailabilityZone,
			}}}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, true)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:              origPackets,
			BytesIn:                origBytes,
			PacketsOut:             replyPackets,
			BytesOut:               replyBytes,
			Proto:                  protocolTCP,
			LocalInstanceID:        localInstance,
			LocalNamespace:         localNamespace,
			LocalNode:              localNode,
			LocalAvailabilityZone:  localAvailabilityZone,
			LocalPod:               localPod,
			LocalIP:                origDstIP,
			LocalPort:              origDstPort,
			LocalApp:               localApp,
			RemoteCloud:            AWS,
			RemoteIP:               origSrcIP,
			RemotePort:             origSrcPort,
			RemoteNode:             remoteNode,
			RemoteAvailabilityZone: remoteAvailabilityZone,
			RemoteInstanceID:       remoteInstance,
			RemoteNamespace:        remoteNamespace,
			RemotePod:              remotePod,
			RemoteApp:              remoteApp,
			RemoteCluster:          "UNKNOWN",
			ConnectionClass:        IntraVPC,
			ConnectionFlags:        make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label TCP flow between pods on the node", func(t *testing.T) {
		t.Parallel()

		var (
			localInstance         = "local_instance"
			remoteInstance        = "remote_instance"
			localNamespace        = "local_namespace"
			remoteNamespace       = "remote_namespace"
			localPod              = "local_pod"
			remotePod             = "remote_pod"
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"
			localApp              = "local_app"
			remoteApp             = "remote_app"
			origSrcIP             = netip.MustParseAddr("10.0.0.1")
			origDstIP             = netip.MustParseAddr("10.0.0.2")
			replySrcIP            = origDstIP
			replyDstIP            = origSrcIP

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		srcPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      localPod,
				Namespace: localNamespace,
				Labels: map[string]string{
					"control-plane-id":       localInstance,
					"app.kubernetes.io/name": localApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: localNode,
			},
		}

		dstPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remotePod,
				Namespace: remoteNamespace,
				Labels: map[string]string{
					"control-plane-id":       remoteInstance,
					"app.kubernetes.io/name": remoteApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: localNode,
			},
		}

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{srcPod}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return([]*corev1.Pod{dstPod}, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": localAvailabilityZone,
			}}}, nil).Times(2)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:              replyPackets,
			BytesIn:                replyBytes,
			PacketsOut:             origPackets,
			BytesOut:               origBytes,
			Proto:                  protocolTCP,
			LocalAvailabilityZone:  localAvailabilityZone,
			LocalNode:              localNode,
			LocalInstanceID:        localInstance,
			LocalNamespace:         localNamespace,
			LocalPod:               localPod,
			LocalIP:                origSrcIP,
			LocalPort:              origSrcPort,
			LocalApp:               localApp,
			RemoteCloud:            AWS,
			RemoteIP:               origDstIP,
			RemotePort:             origDstPort,
			RemoteNode:             localNode,
			RemoteAvailabilityZone: localAvailabilityZone,
			RemoteInstanceID:       remoteInstance,
			RemoteNamespace:        remoteNamespace,
			RemotePod:              remotePod,
			RemoteApp:              remoteApp,
			RemoteCluster:          "UNKNOWN",
			ConnectionClass:        IntraVPC,
			ConnectionFlags:        make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should return an error for an uknown but active flow", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(gomock.Any()).Return(nil, nil).AnyTimes()
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil).AnyTimes()
		mockWatcher.EXPECT().GetNodeByName(gomock.Any()).Return(nil, nil).AnyTimes()

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow("", &grpc.Observation_Flow{
			Proto:    IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{},
			Reply:    &grpc.Observation_Flow_FlowTuple{},
		})

		assert.Error(t, err)
		assert.Nil(t, data)
	})

	t.Run("Should correctly label flow from a pod on the node to a service in AWS", func(t *testing.T) {
		t.Parallel()

		// In AWS, connections to service IPs have the original tuple (podIP,
		// serviceIP) and the reply tuple (podIP, podIP).
		var (
			localAvailabilityZone  = "local_availability_zone"
			localInstance          = "local_instance"
			remoteInstance         = "remote_instance"
			localNamespace         = "local_namespace"
			remoteNamespace        = "remote_namespace"
			localPod               = "local_pod"
			remotePod              = "remote_pod"
			localNode              = "local_node"
			remoteNode             = "remote_node"
			localApp               = "local_app"
			remoteApp              = "remote_app"
			remoteAvailabilityZone = "remote_availability_zone"

			origSrcIP  = netip.MustParseAddr("10.0.0.1")
			origDstIP  = netip.MustParseAddr("10.0.0.2")
			replySrcIP = netip.MustParseAddr("10.3.0.0")
			replyDstIP = origSrcIP

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		srcPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      localPod,
				Namespace: localNamespace,
				Labels: map[string]string{
					"control-plane-id":       localInstance,
					"app.kubernetes.io/name": localApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: localNode,
			},
		}

		dstPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remotePod,
				Namespace: remoteNamespace,
				Labels: map[string]string{
					"control-plane-id":       remoteInstance,
					"app.kubernetes.io/name": remoteApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: remoteNode,
			},
		}

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{srcPod}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return([]*corev1.Pod{dstPod}, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": localAvailabilityZone,
			}}}, nil)
		mockWatcher.EXPECT().GetNodeByName(remoteNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": remoteAvailabilityZone,
			}}}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                AWS,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:              replyPackets,
			BytesIn:                replyBytes,
			PacketsOut:             origPackets,
			BytesOut:               origBytes,
			Proto:                  protocolTCP,
			LocalAvailabilityZone:  localAvailabilityZone,
			LocalNode:              localNode,
			LocalInstanceID:        localInstance,
			LocalNamespace:         localNamespace,
			LocalPod:               localPod,
			LocalIP:                origSrcIP,
			LocalPort:              origSrcPort,
			LocalApp:               localApp,
			RemoteCloud:            AWS,
			RemoteIP:               replySrcIP,
			RemotePort:             replySrcPort,
			RemoteNode:             remoteNode,
			RemoteAvailabilityZone: remoteAvailabilityZone,
			RemoteInstanceID:       remoteInstance,
			RemoteNamespace:        remoteNamespace,
			RemotePod:              remotePod,
			RemoteApp:              remoteApp,
			RemoteCluster:          "UNKNOWN",
			ConnectionClass:        IntraVPC,
			ConnectionFlags:        make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label flow from a pod on the node to a service in GCP and Azure", func(t *testing.T) {
		t.Parallel()

		// In GCP and Azure, connections to service IPs have the original tuple (podIP,
		// podIP) and the reply tuple (podIP, podIP). The service IP is never observed.
		var (
			localInstance          = "local_instance"
			remoteInstance         = "remote_instance"
			localNamespace         = "local_namespace"
			remoteNamespace        = "remote_namespace"
			localPod               = "local_pod"
			remotePod              = "remote_pod"
			localNode              = "local_node"
			remoteNode             = "remote_node"
			localAvailabilityZone  = "local_availability_zone"
			remoteAvailabilityZone = "remote_availability_zone"
			localApp               = "local_app"
			remoteApp              = "remote_app"

			origSrcIP  = netip.MustParseAddr("10.0.0.1")
			origDstIP  = netip.MustParseAddr("10.0.0.2")
			replySrcIP = origDstIP
			replyDstIP = origSrcIP

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		srcPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      localPod,
				Namespace: localNamespace,
				Labels: map[string]string{
					"control-plane-id":       localInstance,
					"app.kubernetes.io/name": localApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: localNode,
			},
		}

		dstPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remotePod,
				Namespace: remoteNamespace,
				Labels: map[string]string{
					"control-plane-id":       remoteInstance,
					"app.kubernetes.io/name": remoteApp,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: remoteNode,
			},
		}

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{srcPod}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return([]*corev1.Pod{dstPod}, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": localAvailabilityZone,
			}}}, nil)

		mockWatcher.EXPECT().GetNodeByName(remoteNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"topology.kubernetes.io/zone": remoteAvailabilityZone,
			}}}, nil)

		noopRemoteLabeler := &RemoteLabeler{
			cloud:                Azure,
			remoteIPPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
		}
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, noopRemoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:              replyPackets,
			BytesIn:                replyBytes,
			PacketsOut:             origPackets,
			BytesOut:               origBytes,
			Proto:                  protocolTCP,
			LocalAvailabilityZone:  localAvailabilityZone,
			LocalNode:              localNode,
			LocalInstanceID:        localInstance,
			LocalNamespace:         localNamespace,
			LocalPod:               localPod,
			LocalIP:                origSrcIP,
			LocalPort:              origSrcPort,
			LocalApp:               localApp,
			RemoteCloud:            Azure,
			RemoteIP:               replySrcIP,
			RemotePort:             replySrcPort,
			RemoteNode:             remoteNode,
			RemoteAvailabilityZone: remoteAvailabilityZone,
			RemoteInstanceID:       remoteInstance,
			RemoteNamespace:        remoteNamespace,
			RemotePod:              remotePod,
			RemoteApp:              remoteApp,
			RemoteCluster:          "UNKNOWN",
			ConnectionClass:        IntraVPC,
			ConnectionFlags:        make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label a connection to a different region in the same cloud", func(t *testing.T) {
		t.Parallel()

		var (
			localCloud         Cloud      = AWS
			localRegion                   = "us-west-2"
			remoteCloud        Cloud      = ""
			remoteRegion                  = ""
			remoteCloudService            = ""
			origDstIP          netip.Addr = netip.IPv4Unspecified()
			replySrcIP         netip.Addr = netip.IPv4Unspecified()
		)

		remoteLabeler, err := NewRemoteLabeler(localRegion, localCloud, Production)
		assert.NoError(t, err)

		for k, v := range remoteLabeler.remoteIPPrefixes {
			if v.cloud == localCloud && v.region == "af-south-1" {
				remoteCloud = v.cloud
				remoteCloudService = v.service
				remoteRegion = v.region
				origDstIP = k.ToAddress().GetNetNetIPAddr()
				replySrcIP = origDstIP
				break
			}
		}

		assert.NotEmpty(t, remoteRegion)
		assert.NotEmpty(t, remoteCloud)
		assert.NotEmpty(t, remoteCloudService)

		// In AWS and GCP, when connecting from a pod to an external
		// destination, the original tuple is (podIP, remoteIP) and the reply
		// tuple (remoteIP, nodeIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			replyDstIP     = netip.MustParseAddr("10.0.0.2")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, remoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           remoteCloud,
			RemoteRegion:          remoteRegion,
			RemoteCloudService:    remoteCloudService,
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       InterRegion,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})

	t.Run("Should correctly label a connection to the same region in the same cloud", func(t *testing.T) {
		t.Parallel()

		var (
			localCloud                    = AWS
			localRegion                   = "us-west-2"
			remoteCloud        Cloud      = ""
			remoteRegion                  = ""
			remoteCloudService            = ""
			origDstIP          netip.Addr = netip.IPv4Unspecified()
			replySrcIP         netip.Addr = netip.IPv4Unspecified()
		)

		remoteLabeler, err := NewRemoteLabeler(localRegion, localCloud, Production)
		assert.NoError(t, err)

		for k, v := range remoteLabeler.remoteIPPrefixes {
			if v.cloud == localCloud && v.region == localRegion {
				remoteCloud = v.cloud
				remoteCloudService = v.service
				remoteRegion = v.region
				origDstIP = k.ToAddress().GetNetNetIPAddr()
				replySrcIP = origDstIP
				break
			}
		}

		assert.NotEmpty(t, remoteRegion)
		assert.NotEmpty(t, remoteCloud)
		assert.NotEmpty(t, remoteCloudService)

		// In AWS and GCP, when connecting from a pod to an external
		// destination, the original tuple is (podIP, remoteIP) and the reply
		// tuple (remoteIP, nodeIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			replyDstIP     = netip.MustParseAddr("10.0.0.2")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, remoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           remoteCloud,
			RemoteRegion:          remoteRegion,
			RemoteCloudService:    remoteCloudService,
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       IntraRegion,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})

	t.Run("Regional subnet allocation is not known for Google Services, their global IPs are interpreted as intra-region", func(t *testing.T) {
		t.Parallel()

		var (
			localCloud                    = GCP
			localRegion                   = "us-east1"
			remoteCloud        Cloud      = ""
			remoteRegion                  = ""
			remoteCloudService            = GoogleService
			origDstIP          netip.Addr = netip.IPv4Unspecified()
			replySrcIP         netip.Addr = netip.IPv4Unspecified()
		)

		remoteLabeler, err := NewRemoteLabeler(localRegion, localCloud, Production)
		assert.NoError(t, err)

		for k, v := range remoteLabeler.remoteIPPrefixes {
			if v.cloud == localCloud && v.service == remoteCloudService {
				remoteRegion = v.region
				remoteCloud = v.cloud
				remoteCloudService = v.service
				origDstIP = k.ToAddress().GetNetNetIPAddr()
				replySrcIP = origDstIP
				if remoteLabeler.findRemoteDetail(ipaddr.NewIPAddressFromNetNetIPAddr(origDstIP).ToIPv4()).service == remoteCloudService {
					// We are looking for a "Google Service" subnet and not a
					// "Google Cloud" subnet (we are testing for Google Service
					// subnets not being region-specific). Sometimes "Google
					// Cloud" has more specific subnets than "Google Service",
					// so we iterate until the actual lookup results in us
					// finding a "Google Service" subnet.
					break
				}
			}
		}

		assert.NotEmpty(t, remoteRegion)
		assert.NotEmpty(t, remoteCloud)
		assert.NotEmpty(t, remoteCloudService)

		// In AWS and GCP, when connecting from a pod to an external
		// destination, the original tuple is (podIP, remoteIP) and the reply
		// tuple (remoteIP, nodeIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			replyDstIP     = netip.MustParseAddr("10.0.0.2")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, remoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           remoteCloud,
			RemoteRegion:          remoteRegion,
			RemoteCloudService:    remoteCloudService,
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       IntraRegion,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})

	t.Run("Regional subnet allocation is known for GCP ranges", func(t *testing.T) {
		t.Parallel()

		var (
			localCloud                    = GCP
			localRegion                   = "us-east1"
			remoteCloud        Cloud      = ""
			remoteRegion                  = ""
			remoteCloudService            = "googlecloud"
			origDstIP          netip.Addr = netip.IPv4Unspecified()
			replySrcIP         netip.Addr = netip.IPv4Unspecified()
		)

		remoteLabeler, err := NewRemoteLabeler(localRegion, localCloud, Production)
		assert.NoError(t, err)

		for k, v := range remoteLabeler.remoteIPPrefixes {
			if v.cloud == localCloud && v.service == remoteCloudService && v.region == "europe-north1" {
				remoteCloud = v.cloud
				remoteRegion = v.region
				origDstIP = k.ToAddress().GetNetNetIPAddr()
				replySrcIP = origDstIP
				break
			}
		}

		// In AWS and GCP, when connecting from a pod to an external
		// destination, the original tuple is (podIP, remoteIP) and the reply
		// tuple (remoteIP, nodeIP).
		var (
			localNode             = "local_node"
			localAvailabilityZone = "local_availability_zone"

			localInstance  = "local_instance"
			localNamespace = "local_namespace"
			localPod       = "local_pod"
			localApp       = "local_app"
			origSrcIP      = netip.MustParseAddr("10.0.0.1")
			replyDstIP     = netip.MustParseAddr("10.0.0.2")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = origDstPort
			replyDstPort uint16 = origSrcPort

			origPackets  uint64 = 10
			origBytes    uint64 = 11
			replyPackets uint64 = 12
			replyBytes   uint64 = 13
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetNodeByInternalIP(origSrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByInternalIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localPod,
					Namespace: localNamespace,
					Labels: map[string]string{
						"control-plane-id":       localInstance,
						"app.kubernetes.io/name": localApp,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: localNode,
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return(nil, nil)
		mockWatcher.EXPECT().GetNodeByName(localNode).Return(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: localNode,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": localAvailabilityZone,
				},
			},
		}, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, remoteLabeler, false)
		data, err := labeler.LabelFlow(localNode, &grpc.Observation_Flow{
			Proto: IP_PROTO_UDP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Packets: origPackets,
				Bytes:   origBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Packets: replyPackets,
				Bytes:   replyBytes,
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, FlowData{
			PacketsIn:             replyPackets,
			BytesIn:               replyBytes,
			PacketsOut:            origPackets,
			BytesOut:              origBytes,
			Proto:                 protocolUDP,
			LocalAvailabilityZone: localAvailabilityZone,
			LocalInstanceID:       localInstance,
			LocalNamespace:        localNamespace,
			LocalNode:             localNode,
			LocalPod:              localPod,
			LocalIP:               origSrcIP,
			LocalPort:             origSrcPort,
			LocalApp:              localApp,
			RemoteCloud:           remoteCloud,
			RemoteRegion:          remoteRegion,
			RemoteCloudService:    "googlecloud",
			RemoteIP:              origDstIP,
			RemotePort:            origDstPort,
			RemoteCluster:         "UNKNOWN",
			ConnectionClass:       InterRegion,
			ConnectionFlags:       make(ConnectionFlags),
		}, *data)
	})
}

func TestGetEndpointsForFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Run("Should return IPs and ports sourced from the original source and reply source when no lookups succeed", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil).AnyTimes()

		var (
			origSrcIP  = netip.MustParseAddr("1.0.0.1")
			origDstIP  = netip.MustParseAddr("1.0.0.2")
			replySrcIP = netip.MustParseAddr("1.0.0.3")
			replyDstIP = netip.MustParseAddr("1.0.0.4")

			origSrcPort  uint16 = 1
			origDstPort  uint16 = 2
			replySrcPort uint16 = 3
			replyDstPort uint16 = 4
		)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)
		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{
			Proto: IP_PROTO_TCP,
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
					Port: uint32(origDstPort),
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, srcEndpointInfo)
		assert.NotNil(t, dstEndpointInfo)
		assert.Equal(t, origSrcIP, srcEndpointInfo.ip)
		assert.Equal(t, replySrcIP, dstEndpointInfo.ip)
		assert.Equal(t, origSrcPort, srcEndpointInfo.port)
		assert.Equal(t, replySrcPort, dstEndpointInfo.port)
	})

	t.Run("Should return an error when GetPodsByIP returns an error", func(t *testing.T) {
		t.Parallel()

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, errFake)
		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)

		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{})
		assert.Error(t, err)
		assert.Nil(t, srcEndpointInfo)
		assert.Nil(t, dstEndpointInfo)
	})

	t.Run("Should be able to establish identity of the source pod in the simple case", func(t *testing.T) {
		t.Parallel()

		var (
			origSrcIP = netip.MustParseAddr("1.0.0.1")
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source pod",
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)
		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, srcEndpointInfo)
		assert.NotNil(t, dstEndpointInfo)
		assert.Equal(t, "source pod", srcEndpointInfo.pod.Name)
	})

	t.Run("Should be able to establish identity of the source pod when NAT is involved", func(t *testing.T) {
		t.Parallel()

		var (
			origSrcIP  = netip.MustParseAddr("1.0.0.1")
			replyDstIP = netip.MustParseAddr("1.0.0.2")
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(origSrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "source pod",
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)
		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, srcEndpointInfo)
		assert.NotNil(t, dstEndpointInfo)
		assert.Equal(t, "source pod", srcEndpointInfo.pod.Name)
	})

	t.Run("Should be able to establish identity of the destination pod in the simple case", func(t *testing.T) {
		t.Parallel()

		var (
			origDstIP  = netip.MustParseAddr("1.0.0.1")
			replySrcIP = origDstIP
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(origDstIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "destination pod",
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)
		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, srcEndpointInfo)
		assert.NotNil(t, dstEndpointInfo)
		assert.Equal(t, "destination pod", dstEndpointInfo.pod.Name)
	})

	t.Run("Should be able to establish identity of the destination pod when it's a service", func(t *testing.T) {
		t.Parallel()

		var (
			origDstIP  = netip.MustParseAddr("1.0.0.1")
			replySrcIP = netip.MustParseAddr("1.0.0.2")
		)

		mockWatcher := mock_watcher.NewWatcher(ctrl)
		mockWatcher.EXPECT().GetPodsByIP(replySrcIP.String()).Return([]*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "destination pod",
				},
			},
		}, nil)
		mockWatcher.EXPECT().GetPodsByIP(gomock.Any()).Return(nil, nil)

		labeler := NewLabeler([]watcher.WatcherInterface{mockWatcher}, nil, false)
		srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(&grpc.Observation_Flow{
			Original: &grpc.Observation_Flow_FlowTuple{
				Destination: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origDstIP.AsSlice())),
					},
				},
			},
			Reply: &grpc.Observation_Flow_FlowTuple{
				Source: &grpc.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &grpc.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replySrcIP.AsSlice())),
					},
				},
			},
		})

		assert.NoError(t, err)
		assert.NotNil(t, srcEndpointInfo)
		assert.NotNil(t, dstEndpointInfo)
		assert.Equal(t, "destination pod", dstEndpointInfo.pod.Name)
	})
}

func TestGetFlowType(t *testing.T) {
	t.Parallel()
	var localNodeName = "node"
	var otherNode = "other"

	t.Run("Should return betweenPodOnNode for traffic between pods on the node", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: localNodeName,
				},
			},
		}, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: localNodeName,
				},
			},
		})

		assert.Equal(t, betweenPodsOnNode, flowType)
	})

	t.Run("Should return fromPodOnNode for traffic from a pod on the node", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: localNodeName,
				},
			},
		}, endpointInfo{})

		assert.Equal(t, flowType, fromPodOnNode)
	})

	t.Run("Should return toPodOnNode for traffic to pod on a node", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{}, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: localNodeName,
				},
			},
		})

		assert.Equal(t, toPodOnNode, flowType)
	})

	t.Run("Should return fromPodOnNode when seeing uknown source and destination that is on another node", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{}, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: otherNode,
				},
			},
		})

		assert.Equal(t, fromPodOnNode, flowType)
	})

	t.Run("Should return toPodOnNode when seeing uknown destination but source that is on another node", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: otherNode,
				},
			},
		}, endpointInfo{})

		assert.Equal(t, toPodOnNode, flowType)
	})

	t.Run("Should return unknown in other cases", func(t *testing.T) {
		t.Parallel()

		labeler := NewLabeler(nil, nil, false)
		flowType := labeler.getFlowType(localNodeName, endpointInfo{}, endpointInfo{})
		assert.Equal(t, unknown, flowType)
	})
}

func TestStringMap(t *testing.T) {
	m := make(ConnectionFlags)
	m[TEST_FLAG] = true
	m[""] = false
	assert.Equal(t, m.String(), "{'':false,'TEST_FLAG':true}")

	m = make(ConnectionFlags)
	assert.Equal(t, m.String(), "{}")
}
