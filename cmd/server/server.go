package main

import (
	"errors"
	"io"
	"time"

	pb "github.com/clickhouse.com/kubenetmon/pkg/grpc"
	"github.com/clickhouse.com/kubenetmon/pkg/inserter"
	"github.com/clickhouse.com/kubenetmon/pkg/labeler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

// Informational metrics about kubenetmon-server.
var (
	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_server_errors_total",
			Help: "Total number of kubenetmon server errors",
		},
		[]string{"type"},
	)

	processedFlowsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_server_processed_observations_total",
			Help: "Number of flows processed by kubenetmon server since start",
		},
		[]string{"type"},
	)

	enqueueWaitHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kubenetmon_server_enqueue_wait_time_us",
			Help:    "Time it took to queue up an observation",
			Buckets: []float64{100, 1000, 10_000, 100_000, 1_000_000, 5_000_000, 10_000_000, 20_000_000},
		},
		[]string{"type"},
	)
)

// flowHandlerServer implements gRPC FlowHandlerServer.
type flowHandlerServer struct {
	pb.UnimplementedFlowHandlerServer

	labeler  labeler.LabelerInterface
	inserter inserter.InserterInterface
}

// NewFlowHandlerServer creates a new flowHandlerServer.
func NewFlowHandlerServer(labeler labeler.LabelerInterface, inserter inserter.InserterInterface) *flowHandlerServer {
	return &flowHandlerServer{
		labeler:  labeler,
		inserter: inserter,
	}
}

// Submit reads from a stream of observation.
func (server *flowHandlerServer) Submit(stream pb.FlowHandler_SubmitServer) error {
	var count uint32

	for {
		observation, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ObservationSummary{
				ObservationCount: count,
			})
		} else if err != nil {
			errorCounter.WithLabelValues([]string{"receive_failed"}...).Inc()
			log.Err(err).Msg("Receive failed")
			return err
		}

		count++
		data, err := server.labeler.LabelFlow(observation.NodeName, observation.Flow)
		if err == nil {
			// Good outcome.
			processedFlowsCounter.WithLabelValues([]string{"labeled"}...).Inc()
			// Queue up the insert. We launch this in a goroutine because it is
			// an async insert that nevertheless blocks
			// (wait_for_async_insert=1). We want to run many such goroutines at
			// once to get more rows to be flushed.
			go func() {
				now := time.Now()
				if err := server.inserter.Queue(inserter.Observation{
					Flow:      *data,
					Timestamp: time.Unix(int64(observation.Timestamp), 0),
				}); err != nil {
					errorCounter.WithLabelValues([]string{"enqueuing_failed"}...).Inc()
					processedFlowsCounter.WithLabelValues([]string{"dropped"}...).Inc()
					enqueueWaitHistogram.WithLabelValues([]string{"failed"}...).Observe(float64(time.Since(now).Microseconds()))
					log.Err(err).Msg("Enqueuing failed")
				} else {
					processedFlowsCounter.WithLabelValues([]string{"enqueued"}...).Inc()
					enqueueWaitHistogram.WithLabelValues([]string{"succeeded"}...).Observe(float64(time.Since(now).Microseconds()))
				}
			}()
		} else if errors.Is(err, labeler.ErrNodeFlow) || errors.Is(err, labeler.ErrIPv6Flow) || errors.Is(err, labeler.ErrIgnoredUDPFlow) {
			// Good outcome.
			processedFlowsCounter.WithLabelValues([]string{"ignored"}...).Inc()
			continue
		} else if errors.Is(err, labeler.ErrInvalidIP) {
			// Bad outcome.
			errorCounter.WithLabelValues([]string{"parse_ip_failed"}...).Inc()
			log.Err(err).Msgf("Failed to parse IP (this should never happen, please investigate) (%v)", observation)
		} else if errors.Is(err, labeler.ErrCannotIdentifykubenetmonirection) {
			errorCounter.WithLabelValues([]string{"labeling_failed"}...).Inc()
			// Technically an error, but happens frequently enough with no
			// consequences that we log it as a warning.
			log.Warn().Msgf("flow direction not clear: %v", err)
		} else {
			// Bad outcome.
			errorCounter.WithLabelValues([]string{"labeling_failed"}...).Inc()
			log.Err(err).Msgf("Failed to label observation (%v)", observation)
		}
	}
}
