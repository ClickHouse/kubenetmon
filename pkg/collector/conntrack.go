package collector

import "github.com/ClickHouse/conntrack"

// ConntrackInterface is a dummy interface used to generate a mock "conntrack"
// implementation used in unit tests.
type ConntrackInterface interface {
	DumpFlowSummaryFilter(filter conntrack.FlowSummaryFilter, opts *conntrack.DumpOptions) ([]conntrack.FlowSummary, error)
}
