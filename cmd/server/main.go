package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	pb "github.com/ClickHouse/kubenetmon/pkg/grpc"
	"github.com/ClickHouse/kubenetmon/pkg/inserter"
	"github.com/ClickHouse/kubenetmon/pkg/labeler"
	"github.com/ClickHouse/kubenetmon/pkg/watcher"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type Config struct {
	MetricsPort                  uint16        `yaml:"metrics_port"`
	GRPCPort                     uint16        `yaml:"grpc_port"`
	Environment                  string        `yaml:"environment"`
	Cloud                        string        `yaml:"cloud"`
	Region                       string        `yaml:"region"`
	Cluster                      string        `yaml:"cluster"`
	MaxGRPCConnectionAge         time.Duration `yaml:"max_grpc_connection_age"`
	NumInserterWorkers           int           `yaml:"num_inserter_workers"`
	IgnoreUDP                    *bool         `yaml:"ignore_udp,omitempty"`
	ClickHouseEnabled            bool          `yaml:"clickhouse_enabled"`
	ClickHouseDatabase           string        `yaml:"clickhouse_database"`
	ClickHouseEndpoint           string        `yaml:"clickhouse_endpoint"`
	ClickHouseDialTimeout        time.Duration `yaml:"clickhouse_dial_timeout"`
	ClickHouseMaxIdleConnections int           `yaml:"clickhouse_max_idle_connections"`
	ClickHouseBatchSize          int           `yaml:"clickhouse_batch_size"`
	ClickHouseBatchSendTimeout   time.Duration `yaml:"clickhouse_batch_send_timeout"`
	ClickHouseWaitForAsyncInsert bool          `yaml:"clickhouse_wait_for_async_insert"`
	ClickHouseSkipPing           bool          `yaml:"clickhouse_skip_ping"`
	ClickHouseDisableTLS         bool          `yaml:"clickhouse_disable_tls"`
	ClickHouseInsecureSkipVerify bool          `yaml:"clickhouse_insecure_skip_verify"`

	ClickHouseUsername string
	ClickHousePassword string
}

const (
	defaultClickHouseConfigPath   string = "/etc/kubenetmon-server/config.yaml"
	defaultClickHouseUsernamePath string = "/etc/clickhouse/username"
	defaultClickHousePasswordPath string = "/etc/clickhouse/password"
)

var configMap = Config{}

func init() {
	b, err := os.ReadFile(defaultClickHouseConfigPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("error reading ClickHouse config at %v", defaultClickHouseConfigPath)
	}

	if err := yaml.Unmarshal(b, &configMap); err != nil {
		log.Fatal().Err(err).Msgf("error unmarshaling ClickHouse config")
	}

	if configMap.IgnoreUDP == nil {
		b := true
		configMap.IgnoreUDP = &b
	}

	username, err := os.ReadFile(defaultClickHouseUsernamePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal().Err(err).Msgf("error reading username file")
	} else if err != nil && errors.Is(err, os.ErrNotExist) {
		log.Warn().Msgf("ClickHouse username is empty")
	}

	password, err := os.ReadFile(defaultClickHousePasswordPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal().Err(err).Msgf("error reading password file")
	} else if err != nil && errors.Is(err, os.ErrNotExist) {
		log.Warn().Msgf("ClickHouse password is empty")
	}

	configMap.ClickHouseUsername = string(username)
	configMap.ClickHousePassword = string(password)
}

func main() {
	log.Info().Msgf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	log.Info().Msgf("GOMEMLIMIT: %d\n", debug.SetMemoryLimit(-1))

	if configMap.MetricsPort == 0 {
		log.Fatal().Err(errors.New("metrics_port should not be empty")).Send()
	}

	if configMap.GRPCPort == 0 {
		log.Fatal().Err(errors.New("grpc_port should not be empty")).Send()
	}

	var environment labeler.Environment
	if configMap.Environment == "" {
		log.Fatal().Err(errors.New("environment should not be empty")).Send()
	}
	environment = labeler.NewEnvironment(configMap.Environment)

	var region string
	if configMap.Region == "" {
		log.Fatal().Err(errors.New("region should not be empty")).Send()
	}
	region = strings.ToLower(configMap.Region)

	if configMap.Cluster == "" {
		log.Fatal().Err(errors.New("cluster")).Send()
	}

	if configMap.MaxGRPCConnectionAge == 0 {
		log.Fatal().Err(errors.New("max_grpc_connection_age should not be empty")).Send()
	}

	if configMap.Cloud == "" {
		log.Fatal().Err(errors.New("cloud should not be empty")).Send()
	}
	var cloud labeler.Cloud
	cloud, err := labeler.NewCloud(configMap.Cloud)
	if err != nil {
		log.Fatal().Err(err).Msg("Can't accept cloud")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", configMap.GRPCPort))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create a listener")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("error getting cluster config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating Clientset")
	}

	localClusterWatcher, err := watcher.NewWatcher(configMap.Cluster, clientset)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Watcher")
	}

	allWatchers := []watcher.WatcherInterface{localClusterWatcher}
	remoteLabeler, err := labeler.NewRemoteLabeler(region, cloud, environment)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Remotelabeler")
	}

	labeler := labeler.NewLabeler(allWatchers, remoteLabeler, *configMap.IgnoreUDP)
	runtimeInfo := inserter.RuntimeInfo{Cloud: cloud, Env: environment, Region: region, Cluster: configMap.Cluster}
	clickhouseOptions := inserter.ClickHouseOptions{
		Database: configMap.ClickHouseDatabase,
		Enabled:  configMap.ClickHouseEnabled,

		Endpoint: configMap.ClickHouseEndpoint,
		Username: configMap.ClickHouseUsername,
		Password: configMap.ClickHousePassword,

		DialTimeout:        configMap.ClickHouseDialTimeout,
		MaxIdleConnections: configMap.ClickHouseMaxIdleConnections,
		BatchSize:          configMap.ClickHouseBatchSize,
		BatchSendTimeout:   configMap.ClickHouseBatchSendTimeout,
		WaitForAsyncInsert: configMap.ClickHouseWaitForAsyncInsert,
		InsecureSkipVerify: configMap.ClickHouseInsecureSkipVerify,

		SkipPing:   configMap.ClickHouseSkipPing,
		DisableTLS: configMap.ClickHouseDisableTLS,
	}

	inserter, err := inserter.NewInserter(clickhouseOptions, runtimeInfo, int(configMap.NumInserterWorkers))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create an inserter")
	}

	server := NewFlowHandlerServer(labeler, inserter)
	go func() {
		opts := []grpc.ServerOption{
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge:      configMap.MaxGRPCConnectionAge,
				MaxConnectionAgeGrace: 1 * time.Minute,
			}),
		}
		grpcServer := grpc.NewServer(opts...)
		pb.RegisterFlowHandlerServer(grpcServer, server)
		log.Info().Msgf("Beginning to serve flowHandlerServer on port :%v\n", configMap.GRPCPort)
		log.Fatal().Err(grpcServer.Serve(listener)).Send()
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Info().Msgf("Beginning to serve metrics on port :%v/metrics\n", configMap.MetricsPort)
	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%v", configMap.MetricsPort), nil)).Send()
}
