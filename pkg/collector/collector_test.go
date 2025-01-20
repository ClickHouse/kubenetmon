package collector

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/ti-mo/conntrack"
	"go.uber.org/mock/gomock"

	mock_collector "github.com/ClickHouse/kubenetmon/pkg/collector/mock"
	pb "github.com/ClickHouse/kubenetmon/pkg/grpc"
	mock_flow_handler "github.com/ClickHouse/kubenetmon/pkg/grpc/mock"
)

var errFake = errors.New("fake error")

func TestNewCollector(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	t.Run("Should fail to create a collector when conntrack dump is erroring out", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return(nil, errFake)
		collector, err := NewCollector(mockConntrack, nil, nil, 15*time.Second, "node", false, 1*time.Second)
		assert.Error(t, err)
		assert.Nil(t, collector)
	})

	t.Run("Should fail to create a collector when conntrack dump has empty counters only", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return([]conntrack.Flow{
			{
				CountersOrig: conntrack.Counter{
					Packets: 0,
					Bytes:   0,
				},
			},
		}, nil)
		collector, err := NewCollector(mockConntrack, nil, nil, 15*time.Second, "node", false, 1*time.Second)
		assert.Error(t, err)
		assert.Nil(t, collector)
	})

	t.Run("Should create a collector", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return([]conntrack.Flow{
			{
				CountersOrig: conntrack.Counter{
					Packets: 0,
					Bytes:   0,
				},
			},
			{
				CountersOrig: conntrack.Counter{
					Packets: 1,
					Bytes:   11,
				},
			},
		}, nil)
		collector, err := NewCollector(mockConntrack, nil, nil, 15*time.Second, "node", false, 1*time.Second)
		assert.NoError(t, err)
		assert.NotNil(t, collector)
	})
}

func TestConntrackCountsNonEmpty(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	t.Run("Should report conntrack counters as empty when they are", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return([]conntrack.Flow{
			{
				CountersOrig: conntrack.Counter{
					Packets: 0,
					Bytes:   0,
				},
			},
			{
				CountersReply: conntrack.Counter{
					Packets: 0,
					Bytes:   0,
				},
			},
		}, nil)

		collector := Collector{
			conntrack: mockConntrack,
		}

		nonEmpty, err := collector.conntrackCountsNonEmpty(1 * time.Second)
		assert.NoError(t, err)
		assert.False(t, nonEmpty)
	})

	t.Run("Should report conntrack counters as non-empty when they are non-empty", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return([]conntrack.Flow{
			{
				CountersOrig: conntrack.Counter{
					Packets: 0,
					Bytes:   0,
				},
			},
			{
				CountersReply: conntrack.Counter{
					Packets: 0,
					Bytes:   1,
				},
			},
		}, nil)

		collector := Collector{
			conntrack: mockConntrack,
		}

		nonEmpty, err := collector.conntrackCountsNonEmpty(1 * time.Second)
		assert.NoError(t, err)
		assert.True(t, nonEmpty)
	})

	t.Run("Should return an error when conntrack interface returns an error", func(t *testing.T) {
		t.Parallel()

		mockConntrack := mock_collector.NewConntrack(ctrl)
		mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return(nil, errFake)

		collector := Collector{
			conntrack: mockConntrack,
		}

		nonEmpty, err := collector.conntrackCountsNonEmpty(1 * time.Second)
		assert.Error(t, err)
		assert.False(t, nonEmpty)
	})
}

func TestShouldIgnoreFlow(t *testing.T) {
	t.Parallel()

	t.Run("Should ignore flows with no data", func(t *testing.T) {
		t.Parallel()

		collector := Collector{}
		shouldIgnore := collector.shouldIgnoreFlow(&conntrack.Flow{
			CountersOrig: conntrack.Counter{
				Packets: 0,
				Bytes:   0,
			},
		})
		assert.True(t, shouldIgnore)
	})

	t.Run("Should ignore non-TCP/UDP protocols", func(t *testing.T) {
		t.Parallel()

		collector := Collector{}
		shouldIgnore := collector.shouldIgnoreFlow(&conntrack.Flow{
			TupleOrig: conntrack.Tuple{
				Proto: conntrack.ProtoTuple{
					Protocol: 123,
				},
			},
			CountersOrig: conntrack.Counter{
				Packets: 1,
				Bytes:   2,
			},
		})
		assert.True(t, shouldIgnore)
	})

	t.Run("Should ignore localhost", func(t *testing.T) {
		t.Parallel()

		collector := Collector{}
		shouldIgnore := collector.shouldIgnoreFlow(&conntrack.Flow{
			TupleOrig: conntrack.Tuple{
				Proto: conntrack.ProtoTuple{
					Protocol: IP_PROTO_TCP,
				},
				IP: conntrack.IPTuple{
					SourceAddress:      netip.MustParseAddr("127.0.0.1"),
					DestinationAddress: netip.MustParseAddr("1.2.3.4"),
				},
			},
			CountersOrig: conntrack.Counter{
				Packets: 1,
				Bytes:   2,
			},
		})
		assert.True(t, shouldIgnore)
	})

	t.Run("Should return an error when Watcher is erroring out", func(t *testing.T) {
		t.Parallel()

		collector := Collector{}
		shouldIgnore := collector.shouldIgnoreFlow(&conntrack.Flow{
			TupleOrig: conntrack.Tuple{
				Proto: conntrack.ProtoTuple{
					Protocol: IP_PROTO_TCP,
				},
				IP: conntrack.IPTuple{
					SourceAddress:      netip.MustParseAddr("1.2.3.4"),
					DestinationAddress: netip.MustParseAddr("1.2.3.4"),
				},
			},
			CountersOrig: conntrack.Counter{
				Packets: 1,
				Bytes:   2,
			},
		})
		assert.False(t, shouldIgnore)
	})

	t.Run("Should not ignore other flows", func(t *testing.T) {
		t.Parallel()

		collector := Collector{}
		shouldIgnore := collector.shouldIgnoreFlow(&conntrack.Flow{
			TupleOrig: conntrack.Tuple{
				Proto: conntrack.ProtoTuple{
					Protocol: IP_PROTO_TCP,
				},
				IP: conntrack.IPTuple{
					SourceAddress:      netip.MustParseAddr("0.0.0.0"),
					DestinationAddress: netip.MustParseAddr("1.2.3.4"),
				},
			},
			TupleReply: conntrack.Tuple{
				IP: conntrack.IPTuple{
					SourceAddress:      netip.MustParseAddr("4.3.2.1"),
					DestinationAddress: netip.MustParseAddr("0.0.0.0"),
				},
			},
			CountersOrig: conntrack.Counter{
				Packets: 1,
				Bytes:   2,
			},
		})
		assert.False(t, shouldIgnore)
	})
}

func TestCollect(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)

	var (
		origSrcIP  = netip.MustParseAddr("1.0.0.1")
		origDstIP  = netip.MustParseAddr("fe80::dead:beef:70:1")
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

		localNode = "node"
	)

	mockConntrack := mock_collector.NewConntrack(ctrl)
	mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: false}).Return([]conntrack.Flow{
		{
			CountersReply: conntrack.Counter{
				Packets: 0,
				Bytes:   1,
			},
		},
	}, nil)
	mockConntrack.EXPECT().Dump(&conntrack.DumpOptions{ZeroCounters: true}).Return([]conntrack.Flow{{
		CountersOrig: conntrack.Counter{
			Packets: origPackets,
			Bytes:   origBytes,
		},
		CountersReply: conntrack.Counter{
			Packets: replyPackets,
			Bytes:   replyBytes,
		},
		TupleOrig: conntrack.Tuple{
			Proto: conntrack.ProtoTuple{
				Protocol:        IP_PROTO_TCP,
				SourcePort:      origSrcPort,
				DestinationPort: origDstPort,
			},
			IP: conntrack.IPTuple{
				SourceAddress:      origSrcIP,
				DestinationAddress: origDstIP,
			},
		},
		TupleReply: conntrack.Tuple{
			Proto: conntrack.ProtoTuple{
				Protocol:        IP_PROTO_TCP,
				SourcePort:      replySrcPort,
				DestinationPort: replyDstPort,
			},
			IP: conntrack.IPTuple{
				SourceAddress:      replySrcIP,
				DestinationAddress: replyDstIP,
			},
		},
	}}, nil)

	now := time.Now()
	mockFlowHandlerClient := mock_flow_handler.NewFlowHandlerClient(ctrl)
	mockFlowHandler_SubmitClient := mock_flow_handler.NewFlowHandler_SubmitClient(ctrl)
	mockFlowHandlerClient.EXPECT().Submit(gomock.Any(), gomock.Any()).Return(mockFlowHandler_SubmitClient, nil)
	mockFlowHandler_SubmitClient.EXPECT().Send(&pb.Observation{
		Flow: &pb.Observation_Flow{
			Proto: uint32(IP_PROTO_TCP),
			Original: &pb.Observation_Flow_FlowTuple{
				Source: &pb.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &pb.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(origSrcIP.AsSlice())),
					},
					Port: uint32(origSrcPort),
				},
				Destination: &pb.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &pb.Observation_Flow_FlowTuple_L4Endpoint_V6{
						V6: origDstIP.AsSlice(),
					},
					Port: uint32(origDstPort),
				},
				Packets: origPackets,
				Bytes:   origBytes,
			},
			Reply: &pb.Observation_Flow_FlowTuple{
				Source: &pb.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &pb.Observation_Flow_FlowTuple_L4Endpoint_V6{
						V6: replySrcIP.AsSlice(),
					},
					Port: uint32(replySrcPort),
				},
				Destination: &pb.Observation_Flow_FlowTuple_L4Endpoint{
					IpAddr: &pb.Observation_Flow_FlowTuple_L4Endpoint_V4{
						V4: uint32(binary.BigEndian.Uint32(replyDstIP.AsSlice())),
					},
					Port: uint32(replyDstPort),
				},
				Packets: replyPackets,
				Bytes:   replyBytes,
			},
		},
		NodeName:  localNode,
		Timestamp: uint64(now.Unix()),
	})
	mockFlowHandler_SubmitClient.EXPECT().CloseAndRecv().Return(&pb.ObservationSummary{ObservationCount: 1}, nil)

	mockClock := mock_collector.NewClock(ctrl)
	mockClock.EXPECT().Now().AnyTimes().Return(now)
	collector, err := NewCollector(mockConntrack, mockFlowHandlerClient, mockClock, 15*time.Second, localNode, false, 1*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, collector)

	err = collector.collectOnce()
	assert.NoError(t, err)
}
