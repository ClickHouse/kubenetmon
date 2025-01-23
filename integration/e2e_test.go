//go:build integration
// +build integration

package integration

import (
	"database/sql"
	"log"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// Run this using Makefile & test/test-kind.sh so that necessary dependencies are spun up.
func TestEndToEnd(t *testing.T) {
	conn, err := sql.Open("clickhouse", "tcp://127.0.0.1:9000?username=default")
	if err != nil {
		t.Fatalf("Failed to connect to ClickHouse: %v", err)
	}
	defer conn.Close()

	var result int
	query := "SELECT sum(bytes) FROM default.network_flows_0"
	err = conn.QueryRow(query).Scan(&result)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result <= 0 {
		t.Fatalf("Recorded bytes should be greater than zero, got %v", result)
	}

	log.Println("Integration test passed!")
}
