package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"

	"clickhouse.com/data-plane-platform/meta"
	yaml "gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	pb "github.com/clickhouse.com/kubenetmon/pkg/grpc"
	"github.com/clickhouse.com/kubenetmon/pkg/inserter"
	"github.com/clickhouse.com/kubenetmon/pkg/labeler"
	"github.com/clickhouse.com/kubenetmon/pkg/watcher"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type ClickHouseConfig struct {
	Database           string        `yaml:"database"`
	Enabled            bool          `yaml:"enabled"`
	Endpoint           string        `yaml:"endpoint"`
	Username           string        `yaml:"username"`
	Password           string        `yaml:"password"`
	DialTimeout        time.Duration `yaml:"dial_timeout"`
	MaxIdleConnections int           `yaml:"max_idle_connections"`
	BatchSize          int           `yaml:"batch_size"`
	BatchSendTimeout   time.Duration `yaml:"batch_send_timeout"`
	WaitForAsyncInsert bool          `yaml:"wait_for_async_insert"`
	SkipPing           bool          `yaml:"skip_ping"`
	DisableTLS         bool          `yaml:"disable_tls"`
	IgnoreUDP          *bool         `yaml:"ignore_udp,omitempty"`
}

const (
	defaultClickHouseConfigPath string = "/etc/kubenetmon-server/clickhouse.yaml"
)

var (
	configMap = ClickHouseConfig{}
)

func init() {
	b, err := os.ReadFile(defaultClickHouseConfigPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("error reading ClickHouse config at %v", defaultClickHouseConfigPath)
	}

	if err := yaml.Unmarshal(b, &configMap); err != nil {
		log.Fatal().Err(err).Msgf("error unmarshaling ClickHouse config")
	}

	if configMap.IgnoreUDP == nil {
		var b = true
		configMap.IgnoreUDP = &b
	}
}

func main() {
	meta.PrintVersionZerolog(&log.Logger, version.Info)
	log.Info().Msgf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	log.Info().Msgf("GOMEMLIMIT: %d\n", debug.SetMemoryLimit(-1))

	metricsPort, ok := os.LookupEnv("METRICS_PORT")
	if !ok {
		log.Fatal().Err(errors.New("METRICS_PORT should not be empty")).Send()
	}

	grpcPort, ok := os.LookupEnv("GRPC_PORT")
	if !ok {
		log.Fatal().Err(errors.New("GRPC_PORT should not be empty")).Send()
	}

	unvalidatedEnvironment, ok := os.LookupEnv("ENVIRONMENT")
	if !ok {
		log.Fatal().Err(errors.New("ENVIRONMENT should not be empty")).Send()
	}
	var environment labeler.Environment
	var err error
	environment, err = labeler.NewEnvironment(unvalidatedEnvironment)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't accept ENVIRONMENT")
	}

	region, ok := os.LookupEnv("REGION")
	if !ok {
		log.Fatal().Err(errors.New("REGION should not be empty")).Send()
	}
	region = strings.ToLower(region)

	clusterType, ok := os.LookupEnv("CLUSTER_TYPE")
	if !ok {
		log.Fatal().Err(errors.New("CLUSTER_TYPE should not be empty")).Send()
	}

	cell, ok := os.LookupEnv("CELL")
	if !ok {
		log.Fatal().Err(errors.New("CELL should not be empty")).Send()
	}

	numInserterWorkersStr, ok := os.LookupEnv("NUM_INSERTER_WORKERS")
	if !ok {
		log.Fatal().Err(errors.New("NUM_INSERTER_WORKERS should not be empty")).Send()
	}
	numInserterWorkers, err := strconv.ParseInt(numInserterWorkersStr, 10, 32)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't parse NUM_INSERTER_WORKERS")
	}

	maxgRPCConnectionAgeSecondsStr, ok := os.LookupEnv("MAX_GRPC_CONNECTION_AGE_SECONDS")
	if !ok {
		log.Fatal().Err(errors.New("MAX_GRPC_CONNECTION_AGE_SECONDS should not be empty")).Send()
	}
	maxgRPCConnectionAgeSeconds, err := strconv.ParseInt(maxgRPCConnectionAgeSecondsStr, 10, 32)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't parse MAX_GRPC_CONNECTION_AGE_SECONDS")
	}

	unvalidatedCloud, ok := os.LookupEnv("CLOUD")
	if !ok {
		log.Fatal().Err(errors.New("CLOUD should not be empty")).Send()
	}
	var cloud labeler.Cloud
	cloud, err = labeler.NewCloud(unvalidatedCloud)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't accept CLOUD")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", grpcPort))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create a listener")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("error getting cluster config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating clientset")
	}

	localClusterWatcher, err := watcher.NewWatcher(clusterType, clientset)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create a Watcher")
	}

	// Create watchers for cluster specified in the kubenetmon-server and prepend
	// them with the local cluster watcher. Watchers are checked one-by-one for
	// labelling, and the first to find the IP is used.
	// watchers, err := createWatchers(clientset)
	allWatchers := []watcher.WatcherInterface{localClusterWatcher}
	remoteLabeler, err := labeler.NewRemoteLabeler(region, cloud, environment)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create remotelabeler")
	}
	var awsAZInfoProvider *labeler.AWSAZInfoProvider
	if cloud == labeler.AWS {
		awsAZInfoProvider, err = labeler.NewAWSAZInfoProvider()
		if err != nil {
			log.Fatal().Err(err).Msgf("cannot initialize aws az info provider")
		}
	}

	labeler := labeler.NewLabeler(allWatchers, remoteLabeler, awsAZInfoProvider, *configMap.IgnoreUDP)
	runtimeInfo := inserter.RuntimeInfo{Cloud: cloud, Env: environment, Region: region, ClusterType: clusterType, Cell: cell}
	clickHouseOptions := inserter.ClickHouseOptions{
		Database: configMap.Database,
		Enabled:  configMap.Enabled,

		Endpoint: configMap.Endpoint,
		Username: configMap.Username,
		Password: configMap.Password,

		DialTimeout:        configMap.DialTimeout,
		MaxIdleConnections: configMap.MaxIdleConnections,
		BatchSize:          configMap.BatchSize,
		BatchSendTimeout:   configMap.BatchSendTimeout,
		WaitForAsyncInsert: configMap.WaitForAsyncInsert,

		SkipPing:   configMap.SkipPing,
		DisableTLS: configMap.DisableTLS,
	}

	inserter, err := inserter.NewInserter(clickHouseOptions, runtimeInfo, int(numInserterWorkers))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create an inserter")
	}

	server := NewFlowHandlerServer(labeler, inserter)
	go func() {
		var opts = []grpc.ServerOption{
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge:      time.Duration(maxgRPCConnectionAgeSeconds) * time.Second,
				MaxConnectionAgeGrace: 1 * time.Minute,
			}),
		}
		grpcServer := grpc.NewServer(opts...)
		pb.RegisterFlowHandlerServer(grpcServer, server)
		log.Info().Msgf("Beginning to serve flowHandlerServer on port :%v\n", grpcPort)
		log.Fatal().Err(grpcServer.Serve(listener)).Send()
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Info().Msgf("Beginning to serve metrics on port :%v/metrics\n", metricsPort)
	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%v", metricsPort), nil)).Send()
}
