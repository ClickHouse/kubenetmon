package collector

import "github.com/ti-mo/conntrack"

// ConntrackInterface is a dummy interface used to generate a mock "conntrack"
// implementation used in unit tests.
type ConntrackInterface interface {
	Dump(opts *conntrack.DumpOptions) ([]conntrack.Flow, error)
}
