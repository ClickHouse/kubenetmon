package inserter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

// Informational metrics about kubenetmon workers.
var (
	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_worker_errors_total",
			Help: "Total number of kubenetmon worker errors",
		},
		[]string{"type"},
	)

	rowsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_worker_rows_total",
			Help: "Number of rows processed by kubenetmon worker since start",
		},
		[]string{"type"},
	)

	batchCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_worker_batches_total",
			Help: "Number of batches processed by kubenetmon worker since start",
		},
		[]string{"type"},
	)

	insertRetryCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "kubenetmon_worker_insert_retries_total",
			Help: "Number of times kubenetmon worker had to retry an insert call",
		},
	)

	insertWaitHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kubenetmon_worker_insert_wait_time_ms",
			Help:    "Time it took to insert a batch, milliseconds",
			Buckets: []float64{1, 100, 1_000, 5_000, 10_000, 60_000, 120_000, 600_000},
		},
		[]string{"type"},
	)

	batchAgeHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "kubenetmon_worker_insert_batch_age_ms",
			Help:    "Inserted batch age at the time of insert query trigger",
			Buckets: []float64{100, 500, 1_000, 5_000, 10_000, 20_000, 30_000},
		},
	)
)

// worker is an internal type that is batching data together for insertion.
type worker struct {
	running bool

	waitAsyncInsert        bool
	conn                   driver.Conn
	batch                  []string
	batchCreationTimestamp time.Time
	batchExpirationTimer   *time.Timer
	batchSendTimeout       time.Duration
	batchSize              int

	runtimeInfo RuntimeInfo
	ch          <-chan Observation
}

func (w *worker) Run() {
	// A small indicator for the top-level Inserter to know if this worker is
	// running or not.
	w.running = true
	defer func(w *worker) {
		w.running = false
	}(w)

	// Loop and do work forever.
	for {
		select {
		case observation, ok := <-w.ch:
			if !ok {
				// Should never happen.
				errorCounter.WithLabelValues([]string{"channel_closed", ""}...).Inc()
				log.Err(errors.New("channel is closed, worker returning")).Send()
				return
			}

			if w.conn != nil {
				w.processRecord(observation)
			}
		case <-w.batchExpirationTimer.C:
			batchSize := len(w.batch)
			if batchSize >= 1 {
				log.Info().Msg("Timer fired, flushing the batch")
				now := time.Now()
				batchAgeHistogram.Observe(float64(w.batchSendTimeout.Milliseconds()))
				if err := w.flush(); err != nil {
					insertWaitHistogram.WithLabelValues([]string{"dropped"}...).Observe(float64(time.Since(now).Milliseconds()))
					errorCounter.WithLabelValues([]string{"flush_batch_failed"}...).Inc()
					rowsCounter.WithLabelValues([]string{"dropped"}...).Add(float64(batchSize))
					batchCounter.WithLabelValues([]string{"dropped"}...).Inc()
					log.Err(err).Msgf("Failed to insert: %v", err)
				} else {
					insertWaitHistogram.WithLabelValues([]string{"inserted"}...).Observe(float64(time.Since(now).Milliseconds()))
					rowsCounter.WithLabelValues([]string{"inserted"}...).Add(float64(batchSize))
					batchCounter.WithLabelValues([]string{"inserted"}...).Inc()
					log.Info().Msg("Inserted batch due to reaching max batch age")
				}
			} else {
				log.Info().Msg("Timer fired but the batch is empty, nothing to flush")
			}
		}
	}
}

func (w *worker) processRecord(observation Observation) {
	// Append data to the batch.
	day := TruncateToStartOfDayUTC(observation.Timestamp).Format("2006-01-02")
	minute := TruncateToStartOfMinuteUTC(observation.Timestamp).Format("2006-01-02 15:04:05")
	connectionFlags := observation.Flow.ConnectionFlags.String()

	if len(w.batch) == 0 {
		// Start the timer on the first append.
		w.batchCreationTimestamp = time.Now()
		resetTimer(w.batchExpirationTimer, w.batchSendTimeout)
	}
	w.batch = append(w.batch,
		fmt.Sprintf("('%v', '%v', %v, '%v', '%v', '%v', %v, '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', %v, %v)",
			day,
			minute,
			60,
			w.runtimeInfo.Env,

			string(observation.Flow.Proto),
			observation.Flow.ConnectionClass,
			connectionFlags,

			"out",

			w.runtimeInfo.Cloud,
			w.runtimeInfo.Region,
			w.runtimeInfo.ClusterType,
			w.runtimeInfo.Cell,
			observation.Flow.LocalAvailabilityZone,
			observation.Flow.LocalNode,
			observation.Flow.LocalInstanceID,
			observation.Flow.LocalNamespace,
			observation.Flow.LocalPod,
			observation.Flow.LocalIP,
			observation.Flow.LocalPort,
			observation.Flow.LocalApp,

			observation.Flow.RemoteCloud,
			observation.Flow.RemoteRegion,
			observation.Flow.RemoteClusterType,
			observation.Flow.RemoteCell,
			observation.Flow.RemoteAvailabilityZone,
			observation.Flow.RemoteNode,
			observation.Flow.RemoteInstanceID,
			observation.Flow.RemoteNamespace,
			observation.Flow.RemotePod,
			observation.Flow.RemoteIP,
			observation.Flow.RemotePort,
			observation.Flow.RemoteApp,
			observation.Flow.RemoteCloudService,

			observation.Flow.BytesOut,
			observation.Flow.PacketsOut))
	w.batch = append(w.batch,
		fmt.Sprintf("('%v', '%v', %v, '%v', '%v', '%v', %v, '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', '%v', %v, %v)",
			day,
			minute,
			60,
			w.runtimeInfo.Env,

			string(observation.Flow.Proto),
			observation.Flow.ConnectionClass,
			connectionFlags,

			"in",

			w.runtimeInfo.Cloud,
			w.runtimeInfo.Region,
			w.runtimeInfo.ClusterType,
			w.runtimeInfo.Cell,
			observation.Flow.LocalAvailabilityZone,
			observation.Flow.LocalNode,
			observation.Flow.LocalInstanceID,
			observation.Flow.LocalNamespace,
			observation.Flow.LocalPod,
			observation.Flow.LocalIP,
			observation.Flow.LocalPort,
			observation.Flow.LocalApp,

			observation.Flow.RemoteCloud,
			observation.Flow.RemoteRegion,
			observation.Flow.RemoteClusterType,
			observation.Flow.RemoteCell,
			observation.Flow.RemoteAvailabilityZone,
			observation.Flow.RemoteNode,
			observation.Flow.RemoteInstanceID,
			observation.Flow.RemoteNamespace,
			observation.Flow.RemotePod,
			observation.Flow.RemoteIP,
			observation.Flow.RemotePort,
			observation.Flow.RemoteApp,
			observation.Flow.RemoteCloudService,

			observation.Flow.BytesIn,
			observation.Flow.PacketsIn))

	batchSize := len(w.batch)
	if batchSize >= w.batchSize {
		now := time.Now()
		batchAgeHistogram.Observe(float64(time.Since(w.batchCreationTimestamp).Milliseconds()))
		if err := w.flush(); err != nil {
			insertWaitHistogram.WithLabelValues([]string{"dropped"}...).Observe(float64(time.Since(now).Milliseconds()))
			errorCounter.WithLabelValues([]string{"flush_batch_failed"}...).Inc()
			rowsCounter.WithLabelValues([]string{"dropped"}...).Add(float64(batchSize))
			batchCounter.WithLabelValues([]string{"dropped"}...).Inc()
			log.Err(err).Msgf("Failed to insert: %v", err)
		} else {
			insertWaitHistogram.WithLabelValues([]string{"inserted"}...).Observe(float64(time.Since(now).Milliseconds()))
			rowsCounter.WithLabelValues([]string{"inserted"}...).Add(float64(batchSize))
			batchCounter.WithLabelValues([]string{"inserted"}...).Inc()
			log.Info().Msg("Inserted batch due to reaching max batch size")
		}
	}
}

func (w *worker) flush() error {
	defer func() {
		w.batch = make([]string, 0, w.batchSize)
	}()

	ctx := clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{"insert_deduplication_token": uuid.New().String()}))
	q := `
INSERT INTO default.network_flows_0
(
	date,
	intervalStartTime,
	intervalSeconds,
	environment,
	proto,
	connectionClass,
	connectionFlags,
	direction,
	localCloud,
	localRegion,
	localClusterType,
	localCell,
	localAvailabilityZone,
	localNode, 
	localInstanceID,
	localNamespace,
	localPod,
	localIPv4,
	localPort,
	localApp,
	remoteCloud,
	remoteRegion,
	remoteClusterType,
	remoteCell,
	remoteAvailabilityZone,
	remoteNode,
	remoteInstanceID,
	remoteNamespace,
	remotePod,
	remoteIPv4,
	remotePort,
	remoteApp,
	remoteCloudService,
	bytes,
	packets
)
VALUES ` + strings.Join(w.batch, ", ")
	if err1 := w.conn.AsyncInsert(ctx, q, w.waitAsyncInsert); err1 != nil {
		insertRetryCounter.Inc()
		log.Err(err1).Msg("failed to insert, going to retry")
		if err2 := w.conn.AsyncInsert(ctx, q, w.waitAsyncInsert); err2 != nil {
			return fmt.Errorf("failed to insert with retry. err1: (%w) err2: (%w); query: (%v)", err1, err2, q)
		}
	}

	return nil
}

func TruncateToStartOfDayUTC(t time.Time) time.Time {
	utcTime := t.UTC()
	return time.Date(utcTime.Year(), utcTime.Month(), utcTime.Day(), 0, 0, 0, 0, time.UTC)
}

func TruncateToStartOfMinuteUTC(t time.Time) time.Time {
	utcTime := t.UTC()
	return time.Date(utcTime.Year(), utcTime.Month(), utcTime.Day(), utcTime.Hour(), utcTime.Minute(), 0, 0, time.UTC)
}

func resetTimer(timer *time.Timer, timeout time.Duration) {
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
	timer.Reset(timeout)
}
