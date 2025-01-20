package labeler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/PraserX/ipconv"
	"github.com/rs/zerolog/log"

	pb "github.com/clickhouse.com/kubenetmon/pkg/grpc"
	"github.com/clickhouse.com/kubenetmon/pkg/watcher"
	corev1 "k8s.io/api/core/v1"
)

// ErrNodeFlow is returned by LabelFlow when the flow has a node (or a
// hostNetwork pod) as one of its endpoints. We ignore such flows because they
// are difficult disambiguate.
var ErrNodeFlow error = errors.New("ignoring flow to or from a node")

// ErrIPv6Flow is returned by LabelFlow when the flow has an IPv6 address for
// one of its endpoints. We currently don't label such flows for simplicity.
var ErrIPv6Flow error = errors.New("ignoring flows with IPv6 endpoints")

// ErrIgnoredUDPFlow is returned if the Labeler is configured to ignore UDP
// flows.
var ErrIgnoredUDPFlow error = errors.New("ignoring UDP flows")

// ErrInvalidIP is returned when an IP from a protobuf message can't be parsed.
var ErrInvalidIP error = errors.New("invalid IP")

// The flow belongs to a connection between either unknown endpoints or
// between two endpoints on some other nodes. Maybe this is a connection
// that was never opened or that is already dying.
//
// Not a problem as long as these warnings don't appear too frequently
// or more than once for any flow.
//
// This can happen also for pods that were starting up but failed to
// start, etc. In this case their connections will linger for a bit in
// conntrack but kubelet will not have information about the pods.
var ErrCannotIdentifykubenetmonirection error = errors.New("cannot identify flow direction")

type ConnectionClass string

// Needed for ClickHouse client to convert the type to string.
func (c ConnectionClass) String() string {
	return string(c)
}

const (
	Unknown        ConnectionClass = "UNKNOWN"
	IntraVPC       ConnectionClass = "INTRA_VPC"
	IntraRegion    ConnectionClass = "INTRA_REGION"
	InterRegion    ConnectionClass = "INTER_REGION"
	PublicInternet ConnectionClass = "PUBLIC_INTERNET"
)

// ConnectionFlag is a type for keys in the ConnectionFlags map.
type ConnectionFlag string

// Needed for ClickHouse client to convert the type to string.
func (f ConnectionFlag) String() string {
	return string(f)
}

type ConnectionFlags map[ConnectionFlag]bool

const TEST_FLAG ConnectionFlag = "TEST_FLAG"

// Needed for ClickHouse client to convert the type to string.
func (m ConnectionFlags) String() string {
	b, _ := json.Marshal(m)
	return strings.ReplaceAll(string(b), "\"", "'")
}

// List of ClickHouse Server ports that are only used by customers. We can bill
// for all traffic on them.
//
// 8124 is used for HTTP with PROXY.
//
// 8443 is used for HTTPS (terminated by ClickHouse).
//
// 9004 is used for MySQL.
//
// 9011 is used by native protocol with PROXY.
//
// 9440 is used by native protocol with TLS (terminated by ClickHouse).
var customerExclusiveClickHouseServerPorts = []uint16{8124, 8443, 9004, 9011, 9440}

// kubenetmonata describes all the information needed for all Prometheus metrics
// related to a conntrack connection (one flow toward the local pod, one flow
// out of the local pod).
type kubenetmonata struct {
	PacketsIn  uint64
	BytesIn    uint64
	PacketsOut uint64
	BytesOut   uint64

	Proto protocol

	LocalAvailabilityZone string
	LocalNode             string
	LocalInstanceID       string
	LocalNamespace        string
	LocalPod              string
	LocalIP               netip.Addr
	LocalPort             uint16
	LocalApp              string

	RemoteCloud Cloud
	// Remote cloud region. If remote is not a k8s pod that kubenetmon server knows
	// of, it will derive it from cloud provider's IP range.
	RemoteRegion           string
	RemoteClusterType      string
	RemoteCell             string
	RemoteAvailabilityZone string
	RemoteNode             string
	RemoteInstanceID       string
	RemoteNamespace        string
	RemotePod              string
	RemoteIP               netip.Addr
	RemotePort             uint16
	RemoteApp              string

	// The cloud service that the remote side associates with based on cloud
	// provider's IP range, e.g. s3.
	RemoteCloudService string

	// Classification of the connection.
	ConnectionClass ConnectionClass

	// Additional flags describing the connection. These can be used for things
	// that are only relevant for some clusters or special cases and therefore
	// don't make sense to have as dedicated columns.
	ConnectionFlags ConnectionFlags
}

type direction string

const (
	Ingress direction = "ingress"
	Egress  direction = "egress"
)

func (d direction) String() string {
	return string(d)
}

// endpointInfo describes one of the two endpoints of a flow.
type endpointInfo struct {
	// The pod endpoint, if any.
	pod *corev1.Pod
	// The IP of the endpoint.
	ip netip.Addr
	// The port of the endpoint.
	port uint16
}

// flowType describes the relationship between the flow direction and the node
// on which it was observed.
type flowType int

const (
	// This flow is from a pod on the node it was observed on.
	fromPodOnNode flowType = iota
	// This flow is to a pod on the node it was observed on.
	toPodOnNode
	// This flow is between pods on the node it was observed on.
	betweenPodsOnNode
	// This pod is between two pods or other endpoints not on the node it was
	// observed on.
	unknown
)

// protocol describes possible flow protocols.
type protocol string

const (
	// TCP protocol.
	IP_PROTO_TCP          = 6
	protocolTCP  protocol = "tcp"

	// UDP protocol.
	IP_PROTO_UDP          = 17
	protocolUDP  protocol = "udp"
)

// LabelerInterface does flow labeling.
type LabelerInterface interface {
	LabelFlow(node string, flow *pb.Observation_Flow) (*kubenetmonata, error)
}

// Labeler implements LabelerInterface.
type Labeler struct {
	// A setting to ignore UDP flows. They represent 1/600 of the traffic but
	// 6/8 of the connection rate, so the ROI on recording them isn't very high.
	// In the future, we can make a more flexible setting to ignore specific
	// protocols, or sample specific protocols while inserting others at 100%
	// resolution, but for now this will do.
	ignoreUDP     bool
	watchers      []watcher.WatcherInterface
	remoteLabeler *RemoteLabeler
	awsAZ         *AWSAZInfoProvider
}

// NewLabeler create a Labeler.
func NewLabeler(watchers []watcher.WatcherInterface, remoteLabeler *RemoteLabeler, awsAZ *AWSAZInfoProvider, ignoreUDP bool) *Labeler {
	return &Labeler{ignoreUDP, watchers, remoteLabeler, awsAZ}
}

func (labeler *Labeler) GetNodeByName(name string) (*corev1.Node, error) {
	for _, watcher := range labeler.watchers {
		if node, err := watcher.GetNodeByName(name); err != nil {
			return nil, err
		} else if node != nil {
			return node, nil
		}
	}

	return nil, nil
}

func (labeler *Labeler) GetNodeByInternalIP(ip string) (*corev1.Node, error) {
	for _, watcher := range labeler.watchers {
		if node, err := watcher.GetNodeByInternalIP(ip); err != nil {
			return nil, err
		} else if node != nil {
			return node, nil
		}
	}

	return nil, nil
}

func (labeler *Labeler) GetPodsByIP(ip string) ([]*corev1.Pod, error) {
	for _, watcher := range labeler.watchers {
		if pod, err := watcher.GetPodsByIP(ip); err != nil {
			return nil, err
		} else if pod != nil {
			return pod, nil
		}
	}

	return nil, nil
}

// labelFlow takes a flow and populates a kubenetmonata struct with all data that
// needs to be reported about the flow.
func (labeler *Labeler) LabelFlow(node string, flow *pb.Observation_Flow) (*kubenetmonata, error) {
	if labeler.ignoreUDP && flow.Proto == IP_PROTO_UDP {
		return nil, ErrIgnoredUDPFlow
	}

	// Here we implicitly check that the IP is parseable, so further on we
	// ignore errors returned by getIP.
	if isIPv6, err := labeler.isIPv6Flow(flow); err != nil {
		return nil, err
	} else if isIPv6 {
		return nil, ErrIPv6Flow
	}

	if isNodeFlow, err := labeler.isNodeFlow(flow); err != nil {
		return nil, err
	} else if isNodeFlow {
		return nil, ErrNodeFlow
	}

	// Retrieve endpoint information, if any.
	srcEndpointInfo, dstEndpointInfo, err := labeler.getEndpointsForFlow(flow)
	if err != nil {
		return nil, err
	}

	data := &kubenetmonata{
		ConnectionFlags: make(map[ConnectionFlag]bool),
	}
	// Set the protocol.
	if flow.GetProto() == IP_PROTO_TCP {
		data.Proto = protocolTCP
	} else if flow.GetProto() == IP_PROTO_UDP {
		data.Proto = protocolUDP
	}

	var localInfo *endpointInfo
	var remoteInfo *endpointInfo
	// Get the type of the flow. Depending on the type, one endpoint will be
	// local and the other will be remote.
	flowType := labeler.getFlowType(node, *srcEndpointInfo, *dstEndpointInfo)
	switch flowType {
	// If two pods on the same node are talking to each other, report the
	// source pod as the local and the destination pod as the remote.
	case fromPodOnNode, betweenPodsOnNode:
		localInfo = srcEndpointInfo
		remoteInfo = dstEndpointInfo

		data.LocalIP = srcEndpointInfo.ip
		data.LocalPort = srcEndpointInfo.port
		data.RemoteIP = dstEndpointInfo.ip
		data.RemotePort = dstEndpointInfo.port

		// The connection is from the local pod, so original counters are "out"
		// and reply counters are "in".
		data.PacketsIn = flow.GetReply().GetPackets()
		data.BytesIn = flow.GetReply().GetBytes()
		data.PacketsOut = flow.GetOriginal().GetPackets()
		data.BytesOut = flow.GetOriginal().GetBytes()

		// Add remote labels based on remote IPs & subnet ip assignment
		if err := labeler.remoteLabeler.labelRemote(dstEndpointInfo, data); err != nil {
			return nil, err
		}
	case toPodOnNode:
		localInfo = dstEndpointInfo
		remoteInfo = srcEndpointInfo

		data.LocalIP = dstEndpointInfo.ip
		data.LocalPort = dstEndpointInfo.port
		data.RemoteIP = srcEndpointInfo.ip
		data.RemotePort = srcEndpointInfo.port

		// The original connection is to the local pod, so original counters are "in"
		// and reply counters are "out".
		data.PacketsIn = flow.GetOriginal().GetPackets()
		data.BytesIn = flow.GetOriginal().GetBytes()
		data.PacketsOut = flow.GetReply().GetPackets()
		data.BytesOut = flow.GetReply().GetBytes()

		// Add remote labels based on remote IPs & subnet ip assignment
		if err := labeler.remoteLabeler.labelRemote(srcEndpointInfo, data); err != nil {
			return nil, err
		}
	case unknown:
		return nil, fmt.Errorf("%w for (origSrc->origDst), (replySrc->replyDst): (%v:%v->%v:%v), (%v:%v->%v:%v)",
			ErrCannotIdentifykubenetmonirection,
			flow.GetOriginal().GetSource().GetIpAddr(),
			flow.GetOriginal().GetSource().GetPort(),
			flow.GetOriginal().GetDestination().GetIpAddr(),
			flow.GetOriginal().GetDestination().GetPort(),
			flow.GetReply().GetSource().GetIpAddr(),
			flow.GetReply().GetSource().GetPort(),
			flow.GetReply().GetDestination().GetIpAddr(),
			flow.GetReply().GetDestination().GetPort(),
		)
	}

	// Set information about the local pod, if there is any.
	if localInfo.pod != nil {
		localPod := localInfo.pod
		data.LocalNamespace = localPod.Namespace
		data.LocalPod = localPod.Name
		data.LocalInstanceID = localPod.Labels["control-plane-id"]
		if localPod.Spec.NodeName != "" {
			data.LocalNode = localPod.Spec.NodeName
			if node, err := labeler.GetNodeByName(localPod.Spec.NodeName); err != nil {
				return nil, err
			} else if node != nil {
				data.LocalAvailabilityZone = node.Labels["topology.kubernetes.io/zone"]
			}
		}
		data.LocalApp = localPod.Labels["app.kubernetes.io/name"]
		if data.LocalApp == "" {
			// Fall back to old-style labels that are still used in some places.
			data.LocalApp = localPod.Labels["k8s-app"]
		}
	}

	// Set information about the remote pod, if there is any.
	if remoteInfo.pod != nil {
		remotePod := remoteInfo.pod
		data.RemoteNamespace = remotePod.Namespace
		data.RemotePod = remotePod.Name
		data.RemoteInstanceID = remotePod.Labels["control-plane-id"]
		if remotePod.Spec.NodeName != "" {
			data.RemoteNode = remotePod.Spec.NodeName
			if node, err := labeler.GetNodeByName(remotePod.Spec.NodeName); err != nil {
				return nil, err
			} else if node != nil {
				data.RemoteAvailabilityZone = node.Labels["topology.kubernetes.io/zone"]
			}
		}
		data.RemoteApp = remotePod.Labels["app.kubernetes.io/name"]
		if data.RemoteApp == "" {
			// Fall back to old-style labels that are still used in some places.
			data.RemoteApp = remotePod.Labels["k8s-app"]
		}
	}

	// For AWS, we try to look at subnet IP assignment directly to figure out
	// local & remote AZ.
	if labeler.awsAZ != nil {
		// If it is AWS, use subnet IP ranges to figure out AZ. First reset the
		// availability zone set previously by node label.
		data.LocalAvailabilityZone = ""
		data.RemoteAvailabilityZone = ""
		// Then try finding the AZ from the AWS AZ info provider.
		localAZ := labeler.awsAZ.findAZDetail(localInfo.ip)
		if localAZ.azID != "" {
			data.LocalAvailabilityZone = localAZ.azID
		}
		if remoteInfo.ip.IsPrivate() || remoteInfo.ip.IsLinkLocalUnicast() || remoteInfo.ip.IsLinkLocalMulticast() || remoteInfo.ip.IsLoopback() {
			remoteAZ := labeler.awsAZ.findAZDetail(remoteInfo.ip)
			if remoteAZ.azID != "" {
				data.RemoteAvailabilityZone = remoteAZ.azID
			}
		}
	}

	data.RemoteCell = "UNKNOWN"
	data.RemoteClusterType = "UNKNOWN"
	return data, nil
}

func (labeler *Labeler) isIPv6Flow(flow *pb.Observation_Flow) (bool, error) {
	if ip, err := getIP(flow.Original.Source); err != nil {
		return false, err
	} else if ip.Is6() {
		return true, nil
	}

	if ip, err := getIP(flow.Original.Destination); err != nil {
		return false, err
	} else if ip.Is6() {
		return true, nil
	}

	if ip, err := getIP(flow.Reply.Source); err != nil {
		return false, err
	} else if ip.Is6() {
		return true, nil
	}

	if ip, err := getIP(flow.Reply.Destination); err != nil {
		return false, err
	} else if ip.Is6() {
		return true, nil
	}

	return false, nil
}

func (labeler *Labeler) isNodeFlow(flow *pb.Observation_Flow) (bool, error) {
	// Ignore flows from a node (original flow is sourced from a node IP).
	srcIP, _ := getIP(flow.GetOriginal().GetSource())
	sourceNode, err := labeler.GetNodeByInternalIP(srcIP.String())
	if err != nil {
		return false, err
	} else if sourceNode != nil {
		return true, nil
	}

	// Ignore flows to a node (reply flow is sourced from a node IP).
	dstIP, _ := getIP(flow.GetReply().GetSource())
	replySourceNode, err := labeler.GetNodeByInternalIP(dstIP.String())
	if err != nil {
		return false, err
	} else if replySourceNode != nil {
		return true, nil
	}

	return false, nil
}

// getEndpointsForFlow finds endpoint information (such as pod identity) for
// ends of a flow. If err is nil, then srcEndpointInfo and dstEndpointInfo are
// non-nil.
func (labeler *Labeler) getEndpointsForFlow(flow *pb.Observation_Flow) (srcEndpointInfo *endpointInfo, dstEndpointInfo *endpointInfo, err error) {
	// Populate things we definitely know already, such as IPs and ports.
	srcIP, _ := getIP(flow.GetOriginal().GetSource())
	srcEndpointInfo = &endpointInfo{
		ip:   srcIP,
		port: uint16(flow.GetOriginal().GetSource().GetPort()),
	}
	dstIP, _ := getIP(flow.GetReply().GetSource())
	dstEndpointInfo = &endpointInfo{
		ip:   dstIP,
		port: uint16(flow.GetReply().GetSource().GetPort()),
	}

	// Try to establish identity of the source pod.
	srcPods, err := labeler.GetPodsByIP(srcEndpointInfo.ip.String())
	if err != nil {
		return nil, nil, err
	} else if len(srcPods) == 1 {
		srcEndpointInfo.pod = srcPods[0]
	} else if len(srcPods) > 1 {
		// Typically never happens.
		return nil, nil, fmt.Errorf("more than one pod maps to origSrc IP %v: %+v", srcEndpointInfo.ip.String(), srcPods)
	}

	// Try to establish identity of the destination pod.
	dstPods, err := labeler.GetPodsByIP(dstEndpointInfo.ip.String())
	if err != nil {
		return nil, nil, err
	} else if len(dstPods) == 1 {
		dstEndpointInfo.pod = dstPods[0]
	} else if len(dstPods) > 1 {
		// This can happen, for example, because some connections were made to
		// hostNetwork pods on a node that has now been shut down. These
		// connections will remain in conntrack for a while longer but there
		// will be no information about the fact that the replySrc is a now-gone
		// node.
		return nil, nil, fmt.Errorf("more than one pod maps to replySrc IP %v: %+v", dstEndpointInfo.ip.String(), dstPods)
	}

	return srcEndpointInfo, dstEndpointInfo, nil
}

// flowType identifies the type of the flow based on its endpoints.
func (labeler *Labeler) getFlowType(localNode string, srcEndpointInfo endpointInfo, dstEndpointInfo endpointInfo) flowType {
	if srcEndpointInfo.pod != nil && srcEndpointInfo.pod.Spec.NodeName == localNode && dstEndpointInfo.pod != nil && dstEndpointInfo.pod.Spec.NodeName == localNode {
		// Both the source and the destination are on the node.
		return betweenPodsOnNode
	} else if srcEndpointInfo.pod != nil && srcEndpointInfo.pod.Spec.NodeName == localNode {
		// The source is on the node.
		return fromPodOnNode
	} else if dstEndpointInfo.pod != nil && dstEndpointInfo.pod.Spec.NodeName == localNode {
		// The destination is on the node.
		return toPodOnNode
	} else if srcEndpointInfo.pod == nil && dstEndpointInfo.pod != nil && dstEndpointInfo.pod.Spec.NodeName != "" {
		// The source is unknown but the destination is on some other node.
		// Assume then that the source is local.
		return fromPodOnNode
	} else if srcEndpointInfo.pod != nil && srcEndpointInfo.pod.Spec.NodeName != "" && dstEndpointInfo.pod == nil {
		// The destination is unknown but the source is on some other node.
		// Assume then that the destination is local.
		return toPodOnNode
	} else {
		// Either both source and destination are some other known nodes or some
		// other unknown nodes. In both cases, we shouldn't be seeing this
		// traffic on the current node.
		return unknown
	}
}

func getIP(endpoint *pb.Observation_Flow_FlowTuple_L4Endpoint) (netip.Addr, error) {
	switch endpoint.GetIpAddr().(type) {
	case *pb.Observation_Flow_FlowTuple_L4Endpoint_V4:
		addr, ok := netip.AddrFromSlice(ipconv.IntToIPv4(endpoint.GetV4()))
		if !ok {
			log.Error().Msgf("could not parse IPv4 address %v", endpoint.GetV4())
			return netip.IPv4Unspecified(), ErrInvalidIP
		}
		return addr, nil
	case *pb.Observation_Flow_FlowTuple_L4Endpoint_V6:
		addr, ok := netip.AddrFromSlice(endpoint.GetV6())
		if !ok {
			log.Error().Msgf("could not parse IPv6 address %v", endpoint.GetV6())
			return netip.IPv6Unspecified(), ErrInvalidIP
		}
		return addr, nil
	default:
		log.Error().Msgf("unknown IP type %v", endpoint.GetIpAddr())
		return netip.IPv4Unspecified(), ErrInvalidIP
	}
}
