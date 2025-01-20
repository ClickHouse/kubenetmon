package labeler

import (
	"fmt"
	"net"
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

type azDetail struct {
	region string
	azID   string
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
	remoteIPPrefixes, remoteIPPrefixesTrie, err := refreshRemoteIPs(aws, gcp, google, azure)
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
			remoteIPPrefixes, remoteIPPrefixesTrie, err := refreshRemoteIPs(aws, gcp, google, azure)
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

func (l *RemoteLabeler) labelRemote(remoteEndpoint *endpointInfo, kubenetmonata *kubenetmonata) error {
	remoteIPAddress := ipaddr.NewIPAddressFromNetNetIPAddr(remoteEndpoint.ip).ToIPv4()
	// If remote endpoint is private, assume it is the same region since right
	// now we don't have cross region peering.
	if remoteIPAddress.IsPrivate() || remoteIPAddress.IsLocal() || remoteIPAddress.IsLoopback() {
		kubenetmonata.RemoteCloud = l.cloud
		kubenetmonata.RemoteRegion = l.region
		kubenetmonata.ConnectionClass = IntraVPC
		return nil
	}

	// Otherwise it is remote IP.
	remoteDetail := l.findRemoteDetail(remoteIPAddress)
	kubenetmonata.RemoteRegion = remoteDetail.region
	kubenetmonata.RemoteCloudService = remoteDetail.service
	kubenetmonata.RemoteCloud = remoteDetail.cloud

	if remoteDetail.cloud == l.cloud {
		if remoteDetail.region == "" {
			errorCounter.WithLabelValues([]string{"intra_cloud_empty_region"}...).Inc()
			return fmt.Errorf("found a connection to an undetermined region in the same %v cloud: remoteEndpoint: (%v), kubenetmonata: (%v)", l.cloud, *remoteEndpoint, *kubenetmonata)
		}

		if (l.cloud == AWS && remoteDetail.region == AmazonGlobalRegion) || (l.cloud == GCP && remoteDetail.region == GoogleGlobalRegion) || (l.cloud == Azure && remoteDetail.region == AzureGlobalRegion) {
			// GCP, AWS, and Azure have "global" IP addresses. For Google, both
			// a Google VM and a Google Service can be "global" (in fact, Google
			// Services are always global). For AWS and Azure, it depends.
			// "Global" IPs are anycast IPs, so we interpret connections to them
			// as staying in the same region (even though it technically can be
			// going to a different region, but we assume the least costly and
			// most likely scenario).
			kubenetmonata.ConnectionClass = IntraRegion
		} else if remoteDetail.region == l.region {
			kubenetmonata.ConnectionClass = IntraRegion
		} else {
			kubenetmonata.ConnectionClass = InterRegion
		}
	} else {
		// If the region is empty, remote is somewhere on the public Internet
		// (or in a different cloud, which is all the same).
		kubenetmonata.ConnectionClass = PublicInternet
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

// getCloudRegions builds a list of regions for the requested cloud.
func getCloudRegions(cloud Cloud, ipRanges interface{}) ([]string, error) {
	var (
		uniqRegions = make(map[string]struct{})
		regions     = make([]string, 0)
	)

	switch cloud {
	case AWS:
		awsIPRanges := ipRanges.(AWSIPRanges)
		for _, p := range awsIPRanges.Prefixes {
			if _, ok := uniqRegions[p.Region]; !ok {
				regions = append(regions, NormalizeCloudString(p.Region))
				uniqRegions[p.Region] = struct{}{}
			}
		}
	case GCP:
		gcpIPRanges := ipRanges.(GCPIPRanges)
		for _, p := range gcpIPRanges.Prefixes {
			if _, ok := uniqRegions[p.Scope]; !ok {
				regions = append(regions, NormalizeCloudString(p.Scope))
				uniqRegions[p.Scope] = struct{}{}
			}
		}
	case Azure:
		azureIPRanges := ipRanges.(AzureIPRanges)
		for _, pg := range azureIPRanges.PrefixGroups {
			region := pg.Properties.Region
			if region == "" {
				region = AzureGlobalRegion
			}

			if _, ok := uniqRegions[region]; !ok {
				regions = append(regions, region)
				uniqRegions[region] = struct{}{}
			}
		}
	}

	return regions, nil
}

// tryGetHostIPs gets all IPs for the host and doesn't return an error when the
// host doesn't exist.
func tryGetHostIPs(host string) ([]net.IP, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		if e, ok := err.(*net.DNSError); ok {
			if e.IsNotFound {
				return nil, nil
			}
		}

		return nil, fmt.Errorf("could not resolve host %v: %w", host, err)
	}

	return ips, nil
}
