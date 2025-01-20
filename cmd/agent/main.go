package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/ti-mo/conntrack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"clickhouse.com/data-plane-platform/meta"

	"github.com/clickhouse.com/kubenetmon/pkg/collector"
	pb "github.com/clickhouse.com/kubenetmon/pkg/grpc"
)

func main() {
	meta.PrintVersionZerolog(&log.Logger, version.Info)
	log.Info().Msgf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	log.Info().Msgf("GOMEMLIMIT: %d\n", debug.SetMemoryLimit(-1))

	nodeName, ok := os.LookupEnv("NODE_NAME")
	if !ok {
		log.Fatal().Err(errors.New("NODE_NAME should not be empty")).Send()
	}

	collectionPeriodString, ok := os.LookupEnv("COLLECTION_INTERVAL")
	if !ok {
		log.Fatal().Err(errors.New("COLLECTION_INTERVAL should not be empty")).Send()
	}
	collectionInterval, err := time.ParseDuration(collectionPeriodString)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't parse COLLECTION_INTERVAL")
	}

	kubenetmonServerHost, ok := os.LookupEnv("kubenetmon_SERVER_SERVICE_HOST")
	if !ok {
		log.Fatal().Err(errors.New("kubenetmon_SERVER_SERVICE_HOST should not be empty")).Send()
	}

	kubenetmonServerPort, ok := os.LookupEnv("kubenetmon_SERVER_SERVICE_PORT")
	if !ok {
		log.Fatal().Err(errors.New("kubenetmon_SERVER_SERVICE_PORT should not be empty")).Send()
	}

	skipConntrackSanityCheckStr, ok := os.LookupEnv("SKIP_CONNTRACK_SANITY_CHECK")
	if !ok {
		log.Fatal().Err(errors.New("SKIP_CONNTRACK_SANITY_CHECK should not be empty")).Send()
	}
	skipConntrackSanityCheck, err := strconv.ParseBool(skipConntrackSanityCheckStr)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't parse SKIP_CONNTRACK_SANITY_CHECK")
	}

	uptimeWaitDurationStr, ok := os.LookupEnv("UPTIME_WAIT_DURATION")
	if !ok {
		log.Fatal().Err(errors.New("UPTIME_WAIT_DURATION should not be empty")).Send()
	}
	uptimeWaitDuration, err := time.ParseDuration(uptimeWaitDurationStr)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't parse UPTIME_WAIT_DURATION")
	}

	conntrackConn, err := conntrack.Dial(nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't create a netlink connection")
	}

	kubenetmonServerAddress := fmt.Sprintf("%v:%v", kubenetmonServerHost, kubenetmonServerPort)
	log.Info().Msgf("Creating kubenetmon-server (%v) gRPC client", kubenetmonServerAddress)
	grpcConn, err := grpc.NewClient(kubenetmonServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal().Err(err).Msg("Can't connect to kubenetmon-server")
	}
	log.Info().Msgf("Connected to kubenetmon-server")

	clock := collector.Clock{}
	flowCollector, err := collector.NewCollector(conntrackConn, pb.NewFlowHandlerClient(grpcConn), clock, collectionInterval, nodeName, skipConntrackSanityCheck, uptimeWaitDuration)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create a collector")
	}

	go func() {
		log.Info().Msg("Starting flow collector")
		flowCollector.Run()
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	metricsPort, ok := os.LookupEnv("METRICS_PORT")
	if !ok {
		log.Fatal().Err(errors.New("METRICS_PORT should not be empty")).Send()
	}

	log.Info().Msgf("Beginning to serve metrics on port :%v/metrics\n", metricsPort)
	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%v", metricsPort), nil)).Send()
}
