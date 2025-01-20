package collector

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	pb "github.com/clickhouse.com/kubenetmon/pkg/grpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
	"github.com/ti-mo/conntrack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	// TCP protocol.
	IP_PROTO_TCP = 6
	// UDP protocol.
	IP_PROTO_UDP = 17
)

var (
	localhost = netip.AddrFrom4([4]byte{127, 0, 0, 1})
)

// Informational metrics about kubenetmon collector.
var (
	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_collector_errors_total",
			Help: "Total number of kubenetmon collector errors",
		},
		[]string{"type"},
	)

	processedFlowsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_collector_processed_flows_total",
			Help: "Number of flows processed by kubenetmon collector since start",
		},
		[]string{"type"},
	)

	collectionDurationMicrosecondsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kubenetmon_collector_last_collection_duration_us",
			Help: "Duration of the last kubenetmon collection run, in microseconds",
		},
	)

	lastSuccessfulCollectionTimestampGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kubenetmon_collector_last_successful_collection_timestamp_unix_seconds",
			Help: "Timestamp of the last successful collection, in Unix seconds",
		},
	)
)

// Collector is an implementation of the Prometheus collector that reports
// flow data.
type Collector struct {
	// conntrack is a connection to the netlink conntrack socket.
	conntrack ConntrackInterface
	// flowHandler is a gRPC client for the FlowHandler service.
	flowHandler pb.FlowHandlerClient
	// clock is an interface that tells current time.
	clock ClockInterface
	// How often Collector should be collecting conntrack data.
	collectionInterval time.Duration
	// localNode is the name of the node the process is running on.
	localNode string
}

// NewCollector creates a new Collector and validates that it can retrieve
// conntrack packet and byte counts.
func NewCollector(conntrack ConntrackInterface, flowHandler pb.FlowHandlerClient, clock ClockInterface, collectionInterval time.Duration, localNode string, skipConntrackSanityCheck bool, uptimeWaitDuration time.Duration) (*Collector, error) {
	collector := Collector{
		conntrack:          conntrack,
		flowHandler:        flowHandler,
		clock:              clock,
		collectionInterval: collectionInterval,
		localNode:          localNode,
	}
	// Do a sanity-check to confirm conntrack accounting is enabled on the node.
	if !skipConntrackSanityCheck {
		if nonEmpty, err := collector.conntrackCountsNonEmpty(uptimeWaitDuration); err != nil {
			return nil, fmt.Errorf("cannot check if conntrack is seeing non-empty counters: %v", err)
		} else if !nonEmpty {
			return nil, errors.New("conntrack is reporting empty flow counters")
		} else {
			log.Info().Msg("Confirmed that kubenetmon can retrieve conntrack packet and byte counters")
		}
	}

	return &collector, nil
}

// Run starts Collector's collection loop.
func (collector *Collector) Run() {
	log.Info().Msgf("Starting collection loop with %v interval", collector.collectionInterval.String())
	ticker := time.NewTicker(collector.collectionInterval)
	for ; true; <-ticker.C {
		now := collector.clock.Now()
		if err := collector.collectOnce(); err != nil {
			log.Error().Err(err).Msg("collectOnce failed")
			errorCounter.WithLabelValues([]string{"collection_failed"}...).Inc()
		} else {
			lastSuccessfulCollectionTimestampGauge.SetToCurrentTime()
		}
		collectionDurationMicrosecondsGauge.Set(float64(time.Since(now).Microseconds()))
	}
}

// collectOnce runs a collection iteration.
func (collector *Collector) collectOnce() error {
	// Create a new connection on every collection iteration. In theory we could
	// reuse connections, but that'd mean we need to instrument connection
	// reopening, so for now let's KISS (keep it simple, stupid, something
	// something).
	stream, err := collector.flowHandler.Submit(context.Background(), grpc.EmptyCallOption{})
	if err != nil {
		return fmt.Errorf("could not create FlowHandler_SubmitClient: %v", err)
	}

	now := collector.clock.Now()
	flows, err := collector.conntrack.Dump(&conntrack.DumpOptions{ZeroCounters: true})
	if err != nil {
		return fmt.Errorf("could not dump conntrack: %v", err)
	}

	for _, flow := range flows {
		if ignore := collector.shouldIgnoreFlow(&flow); ignore {
			processedFlowsCounter.WithLabelValues([]string{"ignored"}...).Inc()
			continue
		}

		originalSourceEndpoint, originalDestinationEndpoint := createEndpoints(flow.TupleOrig)
		replySourceEndpoint, replyDestinationEndpoint := createEndpoints(flow.TupleReply)
		if err := stream.Send(&pb.Observation{
			Flow: &pb.Observation_Flow{
				Proto: uint32(flow.TupleOrig.Proto.Protocol),
				Original: &pb.Observation_Flow_FlowTuple{
					Source:      originalSourceEndpoint,
					Destination: originalDestinationEndpoint,
					Packets:     flow.CountersOrig.Packets,
					Bytes:       flow.CountersOrig.Bytes,
				},
				Reply: &pb.Observation_Flow_FlowTuple{
					Source:      replySourceEndpoint,
					Destination: replyDestinationEndpoint,
					Packets:     flow.CountersReply.Packets,
					Bytes:       flow.CountersReply.Bytes,
				},
			},
			NodeName:  collector.localNode,
			Timestamp: uint64(now.Unix()),
		}); err != nil {
			log.Error().Err(err).Msg("Send observation failed")
			errorCounter.WithLabelValues([]string{"send_failed"}...).Inc()
		} else {
			processedFlowsCounter.WithLabelValues([]string{"sent"}...).Inc()
		}
	}

	if reply, err := stream.CloseAndRecv(); err != nil {
		return fmt.Errorf("received status code %v and error %v when calling CloseAndRecv", status.Code(err).String(), err)
	} else {
		log.Info().Msgf("%v datapoints were accepted through the last stream", reply.GetObservationCount())
	}

	return nil
}

// conntrackCountsNonEmpty performs a sanity check and validates that conntrack
// counters are reporting non-zero numbers on some flows, validating that
// conntrack accounting is enabled on the node.
func (collector *Collector) conntrackCountsNonEmpty(uptimeWaitDuration time.Duration) (bool, error) {
	for {
		// Check conntrack counters.
		flows, err := collector.conntrack.Dump(&conntrack.DumpOptions{ZeroCounters: false})
		if err != nil {
			return false, err
		}

		// If they are not empty, we are all set. Return success.
		for _, flow := range flows {
			if flow.CountersOrig.Packets != 0 ||
				flow.CountersOrig.Bytes != 0 ||
				flow.CountersReply.Packets != 0 ||
				flow.CountersReply.Bytes != 0 {
				return true, nil
			}
		}

		// If conntrack counters are empty, check if the node uptime is less
		// than uptimeWaitDuration. Maybe it's a new node and node-configurator
		// hasn't completed running yet.
		content, err := os.ReadFile("/proc/uptime")
		if err != nil {
			return false, fmt.Errorf("Error reading /proc/uptime: %w", err)
		}

		uptime, _ := strconv.ParseFloat(strings.Split(string(content), " ")[0], 64)
		if uptime >= uptimeWaitDuration.Seconds() {
			// The node has been up for longer than uptimeWaitDuration.
			log.Info().Msgf("System uptime is now %v seconds.", uptime)
			break
		}

		log.Info().Msgf("Current uptime is %v seconds, which is less than %v seconds limit. Waiting for 10 seconds until checking conntrack counters are enabled again...", uptime, uptimeWaitDuration.Seconds())
		time.Sleep(10 * time.Second)
	}

	return false, nil
}

// shouldIgnoreFlow returns true if the flow is to be ignored and not reported
// to the server for labelling, for example because it's a localhost flow.
func (collector *Collector) shouldIgnoreFlow(flow *conntrack.Flow) bool {
	// Ignore flows with no data to report.
	if flow.CountersOrig.Bytes == 0 && flow.CountersOrig.Packets == 0 && flow.CountersReply.Bytes == 0 && flow.CountersReply.Packets == 0 {
		return true
	}

	// Ignore non-TCP/UDP flows.
	if flow.TupleOrig.Proto.Protocol != IP_PROTO_TCP && flow.TupleOrig.Proto.Protocol != IP_PROTO_UDP {
		return true
	}

	// Ignore localhost flows.
	if flow.TupleOrig.IP.SourceAddress == localhost ||
		flow.TupleOrig.IP.DestinationAddress == localhost ||
		flow.TupleReply.IP.SourceAddress == localhost ||
		flow.TupleReply.IP.DestinationAddress == localhost {
		return true
	}

	return false
}

func createEndpoints(tuple conntrack.Tuple) (src *pb.Observation_Flow_FlowTuple_L4Endpoint, dst *pb.Observation_Flow_FlowTuple_L4Endpoint) {
	src = &pb.Observation_Flow_FlowTuple_L4Endpoint{}
	src.Port = uint32(tuple.Proto.SourcePort)
	if tuple.IP.SourceAddress.Is4() {
		src.IpAddr = &pb.Observation_Flow_FlowTuple_L4Endpoint_V4{
			V4: uint32(binary.BigEndian.Uint32(tuple.IP.SourceAddress.AsSlice())),
		}
	} else {
		src.IpAddr = &pb.Observation_Flow_FlowTuple_L4Endpoint_V6{
			V6: tuple.IP.SourceAddress.AsSlice(),
		}
	}

	dst = &pb.Observation_Flow_FlowTuple_L4Endpoint{}
	dst.Port = uint32(tuple.Proto.DestinationPort)
	if tuple.IP.DestinationAddress.Is4() {
		dst.IpAddr = &pb.Observation_Flow_FlowTuple_L4Endpoint_V4{
			V4: uint32(binary.BigEndian.Uint32(tuple.IP.DestinationAddress.AsSlice())),
		}
	} else {
		dst.IpAddr = &pb.Observation_Flow_FlowTuple_L4Endpoint_V6{
			V6: tuple.IP.DestinationAddress.AsSlice(),
		}
	}

	return src, dst
}
