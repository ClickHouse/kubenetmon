package inserter

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ClickHouse/kubenetmon/pkg/labeler"
)

// ErrInsertTimeout is returned by Inserter.Insert.
var ErrInsertTimeout error = errors.New("insert timed out")

// RuntimeInfo has some basic information about where this inserter is running.
// This observation gets inserted into ClickHouse as local endpoint annotations.
type RuntimeInfo struct {
	Cloud   labeler.Cloud
	Env     labeler.Environment
	Region  string
	Cluster string
}

// ClickHouseOptions are for configuring the ClickHouse connection.
type ClickHouseOptions struct {
	// Database name.
	Database string
	// Whether to enable this connection.
	Enabled bool
	// ClickHouse endpoint.
	Endpoint string
	// ClickHouse username.
	Username string
	// ClickHouse password.
	Password string

	// Timeout for dialing a new connection.
	DialTimeout time.Duration
	// Max idle connections pooled.
	MaxIdleConnections int
	// Batch size.
	BatchSize int
	// How soon the batch is sent if it's incomplete.
	BatchSendTimeout time.Duration
	// wait_for_async_insert setting
	WaitForAsyncInsert bool

	// Skip ClickHouse ping on startup.
	SkipPing bool
	// Disable TLS on ClickHouse connection.
	DisableTLS bool
	// Allow TLS with unverified certificates
	InsecureSkipVerify bool
}

// Observation is a conntrack observation from kubenetmon-agent labeled by the
// labeler.
type Observation struct {
	Flow      labeler.FlowData
	Timestamp time.Time
}

// InserterInterface inserts network observation into ClickHouse.
type InserterInterface interface {
	Queue(observation Observation) error
}

// Inserter implements InserterInterface.
type Inserter struct {
	// Channel over which Inserter sends Observations to workers for insertion.
	ch chan<- Observation
	// A list of workers batching observation together.
	workers []*worker
}

// NewInserter creates a new Inserter.
func NewInserter(clickhouseOptions ClickHouseOptions, runtimeInfo RuntimeInfo, numWorkers int) (*Inserter, error) {
	ctx := context.Background()
	conn, err := createClickHouseConnectionWithOptions(ctx, clickhouseOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	// Create workers. Each worker is batching observations together in
	// BatchSize batches. All workers compete to receive Observations from the
	// same channel. The channel has a buffer of numWorkers * batchSize capacity
	// in case workers get stuck.
	ch := make(chan Observation, numWorkers*clickhouseOptions.BatchSize)
	workers := make([]*worker, 0, numWorkers)
	for range numWorkers {
		w := worker{
			waitAsyncInsert:      clickhouseOptions.WaitForAsyncInsert,
			conn:                 conn,
			batch:                make([]string, 0, clickhouseOptions.BatchSize),
			batchExpirationTimer: time.NewTimer(clickhouseOptions.BatchSendTimeout),
			batchSendTimeout:     clickhouseOptions.BatchSendTimeout,
			batchSize:            clickhouseOptions.BatchSize,

			runtimeInfo: runtimeInfo,
			ch:          ch,
		}
		workers = append(workers, &w)
		go w.Run()
	}

	return &Inserter{ch, workers}, nil
}

func createClickHouseConnectionWithOptions(ctx context.Context, clickhouseOptions ClickHouseOptions) (driver.Conn, error) {
	if !clickhouseOptions.Enabled {
		return nil, nil
	}

	options := clickhouse.Options{
		Addr: []string{clickhouseOptions.Endpoint},
		Auth: clickhouse.Auth{
			Database: clickhouseOptions.Database,
			Username: clickhouseOptions.Username,
			Password: clickhouseOptions.Password,
		},
		ClientInfo: clickhouse.ClientInfo{
			Products: []struct {
				Name    string
				Version string
			}{
				{Name: "kubenetmon-server"},
			},
		},
		MaxIdleConns: clickhouseOptions.MaxIdleConnections,
		DialTimeout:  clickhouseOptions.DialTimeout,
	}
	// Configure TLS if need be.
	if !clickhouseOptions.DisableTLS {
		options.TLS = &tls.Config{InsecureSkipVerify: clickhouseOptions.InsecureSkipVerify}
	}

	conn, err := clickhouse.Open(&options)
	if err != nil {
		return nil, err
	}

	// Test ClickHouse connection.
	if !clickhouseOptions.SkipPing {
		if err := conn.Ping(ctx); err != nil {
			if exception, ok := err.(*clickhouse.Exception); ok {
				err = fmt.Errorf("exception [%d] %s\n%s", exception.Code, exception.Message, exception.StackTrace)
			}
			return nil, err
		}
	}

	return conn, nil
}

// Queue sends the observation to the first available worker for batching. It
// does not block unless the insertion queue is filled up, which is unlikely and
// indicates we have a problem. It returns ErrInsertTimeout if the queue did not
// free up in ErrInsertTimeout.
func (inserter *Inserter) Queue(observation Observation) error {
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		return ErrInsertTimeout
	case inserter.ch <- observation:
		// Free up timer resources by draining the channel. If Stop returns
		// True, the channel does not need draining.
		if !timer.Stop() {
			<-timer.C
		}
	}

	return nil
}
