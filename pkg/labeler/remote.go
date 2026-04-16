package labeler

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
	"github.com/seancfoley/ipaddress-go/ipaddr"
)

// RemoteLabeler implements the labeling on remote endpoint based on remote IPs.
// RemoteLabeler has knowledge about cloud provider IP ranges (GCP, Azure, AWS)
//
// The label that RemoteLabeler could populate are:
//   - RemoteRegion, in case kubenetmon cannot figure it out based on k8s info
//   - RemoteCloudService, the cloud services that the remote associates with, e.g. s3
//   - Classification of the flow
type RemoteLabeler struct {
	// remoteIPPrefixes is a map from ip prefix to cloud service detail
	remoteIPPrefixes map[ipaddr.IPv4AddressKey]remoteIPPrefixDetail
	// remoteIPPrefixesTrie is a ip prefix trie for fast look up
	remoteIPPrefixesTrie *ipaddr.IPv4AddressTrie
	remoteIPPrefixMu     sync.RWMutex

	// region & cloud that kubenetmon server is running in
	environment Environment
	region      string
	cloud       Cloud
}

var (
	publicIPRefreshCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_public_ip_refreshes_total",
			Help: "Total number of public IP range refreshes",
		},
		[]string{"result"},
	)

	errorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubenetmon_remote_labeler_errors_total",
			Help: "Total number of kubenetmon remote labeler errors",
		},
		[]string{"type"},
	)
)

func NewRemoteLabeler(localRegion string, localCloud Cloud, environment Environment) (*RemoteLabeler, error) {
	// Fetch the IP ranges immediately at initialization.
	aws, gcp, google, azure, err := getCloudRanges()
	if err != nil {
		errorCounter.WithLabelValues([]string{"get_cloud_ranges"}...).Inc()
		return nil, fmt.Errorf("error fetching cloud ranges: %v", err)
	}
	custom, err := getCustomRanges()
	if err != nil {
		errorCounter.WithLabelValues([]string{"get_custom_ranges"}...).Inc()
		return nil, fmt.Errorf("error fetching custom ranges: %v", err)
	}
	remoteIPPrefixes, remoteIPPrefixesTrie, err := refreshRemoteIPs(aws, gcp, google, azure, custom)
	if err != nil {
		errorCounter.WithLabelValues([]string{"remote_ips_refresh"}...).Inc()
		publicIPRefreshCounter.WithLabelValues([]string{"failed"}...).Inc()
		return nil, err
	}
	publicIPRefreshCounter.WithLabelValues([]string{"succeeded"}...).Inc()
	log.Info().Msgf("RemoteLabeler initialized with %v prefixes", len(remoteIPPrefixes))

	// Make a sanity check that the region we are configured with exists in the
	// list of fetched prefixes.
	var (
		regionExists bool
		uniqRegions  = make(map[string]struct{})
		regions      = []string{}
	)
	for _, detail := range remoteIPPrefixes {
		if localRegion == detail.region {
			regionExists = true
			break
		}

		if _, ok := uniqRegions[detail.region]; !ok {
			uniqRegions[detail.region] = struct{}{}
			regions = append(regions, detail.region)
		}
	}
	if !regionExists {
		return nil, fmt.Errorf("won't be able to determine intra-region traffic correctly: local region %v not found among fetched regional prefixes (%v)", localRegion, regions)
	}

	remoteLabeler := &RemoteLabeler{
		remoteIPPrefixes:     remoteIPPrefixes,
		remoteIPPrefixesTrie: remoteIPPrefixesTrie,
		environment:          environment,
		region:               localRegion,
		cloud:                localCloud,
	}

	// Kick off a daily public IP refresh goroutine.
	publicIPTicker := time.NewTicker(24 * time.Hour)
	go func() {
		for range publicIPTicker.C {
			aws, gcp, google, azure, err := getCloudRanges()
			if err != nil {
				errorCounter.WithLabelValues([]string{"get_cloud_ranges"}...).Inc()
				publicIPRefreshCounter.WithLabelValues([]string{"failed"}...).Inc()
				log.Error().Err(err).Msg("failed to refresh cloud provider IPs")
				continue
			}
			custom, err := getCustomRanges()
			if err != nil {
				errorCounter.WithLabelValues([]string{"get_custom_ranges"}...).Inc()
				publicIPRefreshCounter.WithLabelValues([]string{"failed"}...).Inc()
				log.Error().Err(err).Msg("failed to refresh custom provider IPs")
				continue
			}
			remoteIPPrefixes, remoteIPPrefixesTrie, err := refreshRemoteIPs(aws, gcp, google, azure, custom)
			if err != nil {
				errorCounter.WithLabelValues([]string{"remote_ips_refresh"}...).Inc()
				publicIPRefreshCounter.WithLabelValues([]string{"failed"}...).Inc()
				log.Error().Err(err).Msg("failed to refresh cloud provider IPs")
				continue
			}

			log.Info().Msgf("remote labeler refreshed with %v prefixes", len(remoteIPPrefixes))
			publicIPRefreshCounter.WithLabelValues([]string{"succeeded"}...).Inc()

			// Update the remote ip prefixes in the remote labeler
			remoteLabeler.remoteIPPrefixMu.Lock()
			remoteLabeler.remoteIPPrefixes = remoteIPPrefixes
			remoteLabeler.remoteIPPrefixesTrie = remoteIPPrefixesTrie
			remoteLabeler.remoteIPPrefixMu.Unlock()
		}
	}()

	return remoteLabeler, nil
}

func (l *RemoteLabeler) labelRemote(remoteEndpoint *endpointInfo, FlowData *FlowData) error {
	remoteIPAddress := ipaddr.NewIPAddressFromNetNetIPAddr(remoteEndpoint.ip).ToIPv4()
	// If remote endpoint is private, assume it is the same region since right
	// now we don't have cross region peering.
	if remoteIPAddress.IsPrivate() || remoteIPAddress.IsLocal() || remoteIPAddress.IsLoopback() {
		FlowData.RemoteCloud = l.cloud
		FlowData.RemoteRegion = l.region
		FlowData.ConnectionClass = IntraVPC
		return nil
	}

	// Otherwise it is remote IP.
	remoteDetail := l.findRemoteDetail(remoteIPAddress)
	FlowData.RemoteRegion = remoteDetail.region
	FlowData.RemoteCloudService = remoteDetail.service
	FlowData.RemoteCloud = remoteDetail.cloud
	FlowData.RemoteAvailabilityZone = remoteDetail.az

	if remoteDetail.cloud == l.cloud {
		if remoteDetail.region == "" {
			errorCounter.WithLabelValues([]string{"intra_cloud_empty_region"}...).Inc()
			return fmt.Errorf("found a connection to an undetermined region in the same %v cloud: remoteEndpoint: (%v), FlowData: (%v)", l.cloud, *remoteEndpoint, *FlowData)
		}

		if (l.cloud == AWS && remoteDetail.region == AmazonGlobalRegion) || (l.cloud == GCP && remoteDetail.region == GoogleGlobalRegion) || (l.cloud == Azure && remoteDetail.region == AzureGlobalRegion) {
			// GCP, AWS, and Azure have "global" IP addresses. For Google, both
			// a Google VM and a Google Service can be "global" (in fact, Google
			// Services are always global). For AWS and Azure, it depends.
			// "Global" IPs are anycast IPs, so we interpret connections to them
			// as staying in the same region (even though it technically can be
			// going to a different region, but we assume the least costly and
			// most likely scenario).
			FlowData.ConnectionClass = IntraRegion
		} else if remoteDetail.region == l.region {
			FlowData.ConnectionClass = IntraRegion
		} else {
			FlowData.ConnectionClass = InterRegion
		}
	} else {
		// If the region is empty, remote is somewhere on the public Internet
		// (or in a different cloud, which is all the same).
		FlowData.ConnectionClass = PublicInternet
	}

	return nil
}

func (l *RemoteLabeler) findRemoteDetail(addr *ipaddr.IPv4Address) remoteIPPrefixDetail {
	l.remoteIPPrefixMu.RLock()
	defer l.remoteIPPrefixMu.RUnlock()
	triePath := l.remoteIPPrefixesTrie.ElementsContaining(addr)
	if triePath.Count() > 0 {
		return l.remoteIPPrefixes[triePath.LongestPrefixMatch().GetKey().ToKey()]
	}

	return remoteIPPrefixDetail{}
}

func getCloudRanges() (awsIPRanges AWSIPRanges, gcpIPRanges GCPIPRanges, googleIPRanges GoogleIPRanges, azureIPRanges AzureIPRanges, err error) {
	err = fetchAndParse("https://ip-ranges.amazonaws.com/ip-ranges.json", &awsIPRanges)
	if err != nil {
		return awsIPRanges, gcpIPRanges, googleIPRanges, azureIPRanges, fmt.Errorf("error fetching AWS IP ranges: %v", err)
	}

	err = fetchAndParse("https://www.gstatic.com/ipranges/cloud.json", &gcpIPRanges)
	if err != nil {
		return awsIPRanges, gcpIPRanges, googleIPRanges, azureIPRanges, fmt.Errorf("error fetching GCP IP ranges: %v", err)
	}

	err = fetchAndParse("https://www.gstatic.com/ipranges/goog.json", &googleIPRanges)
	if err != nil {
		return awsIPRanges, gcpIPRanges, googleIPRanges, azureIPRanges, fmt.Errorf("error fetching Google IP ranges: %v", err)
	}

	azureIPRanges, err = getAzureRanges()
	if err != nil {
		return awsIPRanges, gcpIPRanges, googleIPRanges, azureIPRanges, fmt.Errorf("error fetching Azure IP ranges: %v", err)
	}

	return
}

func getCustomRanges() (customIPRanges CustomIPRanges, err error) {
	body, err := os.ReadFile("/etc/kubenetmon/custom_ranges.json")
	if err != nil {
		return customIPRanges, err
	}

	err = json.Unmarshal(body, &customIPRanges)
	if err != nil {
		return customIPRanges, err
	}

	return customIPRanges, err
}
