package labeler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/seancfoley/ipaddress-go/ipaddr"
)

const (
	AmazonService       = "amazon"
	AmazonS3            = "s3"
	AmazonGlobalRegion  = "global"
	GoogleService       = "googleservice"
	GoogleGlobalRegion  = "global"
	AzureStorageService = "azurestorage"
	AzureGlobalRegion   = "global"
	AzureCloudService   = "azurecloud"
	AzureService        = "azureservice"
)

type Environment string

const (
	Development Environment = "development"
	Staging     Environment = "staging"
	Production  Environment = "production"
)

// NewEnvironment creates and validates the Environment name from a string.
func NewEnvironment(environment string) (Environment, error) {
	switch environment {
	case Development.String():
		return Development, nil
	case Staging.String():
		return Staging, nil
	case Production.String():
		return Production, nil
	}

	return "", fmt.Errorf("unknown environment %v", environment)
}

func (e Environment) String() string {
	return string(e)
}

type Cloud string

const (
	AWS   Cloud = "aws"
	GCP   Cloud = "gcp"
	Azure Cloud = "azure"
)

// NewCloud creates and validates the Cloud name from a string.
func NewCloud(cloud string) (Cloud, error) {
	switch cloud {
	case AWS.String():
		return AWS, nil
	case GCP.String():
		return GCP, nil
	case Azure.String():
		return Azure, nil
	}

	return "", fmt.Errorf("unknown cloud %v", cloud)
}

func (c Cloud) String() string {
	return string(c)
}

/* AWS IP range structs */
type AWSIPRanges struct {
	SyncToken    string          `json:"syncToken"`
	CreateDate   string          `json:"createDate"`
	Prefixes     []AWSPrefix     `json:"prefixes"`
	IPv6Prefixes []AWSIPv6Prefix `json:"ipv6_prefixes"`
}
type AWSPrefix struct {
	IPPrefixStr        string `json:"ip_prefix"`
	Region             string `json:"region"`
	Service            string `json:"service"`
	NetworkBorderGroup string `json:"network_border_group"`
}
type AWSIPv6Prefix struct {
	IPv6PrefixStr      string `json:"ipv6_prefix"`
	Region             string `json:"region"`
	Service            string `json:"service"`
	NetworkBorderGroup string `json:"network_border_group"`
}

/* GCP IP Range structs */
type GCPIPRanges struct {
	SyncToken    string      `json:"syncToken"`
	CreationTime string      `json:"creationTime"`
	Prefixes     []GCPPrefix `json:"prefixes"`
}
type GCPPrefix struct {
	IPv4PrefixStr string `json:"ipv4Prefix,omitempty"`
	IPv6PrefixStr string `json:"ipv6Prefix,omitempty"`
	Service       string `json:"service"`
	Scope         string `json:"scope"` // region
}

/* Google IP range structs */
type GoogleIPRange struct {
	IPv4PrefixStr string `json:"ipv4Prefix,omitempty"`
	IPv6PrefixStr string `json:"ipv6Prefix,omitempty"`
}
type GoogleIPRanges struct {
	SyncToken    string          `json:"syncToken"`
	CreationTime string          `json:"creationTime"`
	Prefixes     []GoogleIPRange `json:"prefixes"`
}

/* Azure IP range structs */
type AzureIPRanges struct {
	ChangeNumber int                `json:"changeNumber"`
	Cloud        string             `json:"cloud"`
	PrefixGroups []AzurePrefixGroup `json:"values"`
}
type AzurePrefixGroup struct {
	Name       string                     `json:"name"`
	ID         string                     `json:"id"`
	Properties AzurePrefixGroupProperties `json:"properties"`
}
type AzurePrefixGroupProperties struct {
	ChangeNumber    int      `json:"changeNumber"`
	Region          string   `json:"region"`
	RegionID        int      `json:"regionId"`
	Platform        string   `json:"platform"`
	SystemService   string   `json:"systemService"`
	AddressPrefixes []string `json:"addressPrefixes"`
	NetworkFeatures []string `json:"networkFeatures"`
}

type remoteIPPrefixDetail struct {
	cloud   Cloud
	service string
	region  string
}

func (d *remoteIPPrefixDetail) Normalize() {
	d.cloud = Cloud(NormalizeCloudString(d.cloud.String()))
	d.service = NormalizeCloudString(d.service)
	d.region = NormalizeCloudString(d.region)
}

// Sometimes AWS advertises a single prefix under multiple services at a time,
// in which case we need to choose which service to attribute it to
// (consistently). We care that we attribute traffic to S3 where possible so
// that in ambiguous situations we mark customer's outbound traffic as going
// through VPC Endpoint rather than NAT Gateway (rounding our pricing and
// charges down rather than up). This is a simple heuristic but it should work.
//
// By convention, lower priority number = higher priority.
var awsServicePriorities map[string]int = map[string]int{
	// Anything else: 0.
	"s3":     -1,
	"amazon": 1,
	"ec2":    2,
}

func refreshRemoteIPs(aws AWSIPRanges, gcp GCPIPRanges, google GoogleIPRanges, azure AzureIPRanges) (map[ipaddr.IPv4AddressKey]remoteIPPrefixDetail, *ipaddr.IPv4AddressTrie, error) {
	remoteIPRanges := make(map[ipaddr.IPv4AddressKey]remoteIPPrefixDetail)
	trie := ipaddr.NewIPv4AddressTrie()

	// Parse the AWS prefix, we only care about IPv4 right now.
	for _, prefix := range aws.Prefixes {
		ip, err := ipaddr.NewIPAddressString(prefix.IPPrefixStr).ToAddress()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid IPv4 address %s", prefix.IPPrefixStr)
		}
		trie.Add(ip.ToIPv4())
		if detail, ok := remoteIPRanges[ip.ToIPv4().ToKey()]; !ok {
			d := remoteIPPrefixDetail{
				cloud:   AWS,
				service: prefix.Service,
				region:  prefix.Region,
			}
			d.Normalize()
			remoteIPRanges[ip.ToIPv4().ToKey()] = d
		} else {
			newServicePrio, ok := awsServicePriorities[NormalizeCloudString(prefix.Service)]
			if !ok {
				newServicePrio = 0
			}

			savedServicePrio, ok := awsServicePriorities[NormalizeCloudString(detail.service)]
			if !ok {
				savedServicePrio = 0
			}

			// Overwrite the detail with the higher-priority service info.
			if newServicePrio <= savedServicePrio {
				d := remoteIPPrefixDetail{
					cloud:   AWS,
					service: prefix.Service,
					region:  prefix.Region,
				}
				d.Normalize()
				remoteIPRanges[ip.ToIPv4().ToKey()] = d
			}
		}
	}

	// Parse the prefix, we only care about IPv4 right now.
	for _, prefix := range gcp.Prefixes {
		if prefix.IPv4PrefixStr == "" {
			continue
		}

		ip, err := ipaddr.NewIPAddressString(prefix.IPv4PrefixStr).ToAddress()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid IPv4 address %s", prefix.IPv4PrefixStr)
		}

		trie.Add(ip.ToIPv4())
		d := remoteIPPrefixDetail{
			cloud:   GCP,
			service: prefix.Service,
			region:  prefix.Scope,
		}
		d.Normalize()
		remoteIPRanges[ip.ToIPv4().ToKey()] = d
	}

	// Parse the prefix, we only care about IPv4 right now.
	for _, prefix := range google.Prefixes {
		if prefix.IPv4PrefixStr == "" {
			continue
		}

		ip, err := ipaddr.NewIPAddressString(prefix.IPv4PrefixStr).ToAddress()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid IPv4 address %s", prefix.IPv4PrefixStr)
		}

		trie.Add(ip.ToIPv4())

		d := remoteIPPrefixDetail{
			cloud:   GCP,
			service: GoogleService,
			region:  GoogleGlobalRegion,
		}
		d.Normalize()
		remoteIPRanges[ip.ToIPv4().ToKey()] = d
	}

	for _, pg := range azure.PrefixGroups {
		region := pg.Properties.Region
		if region == "" {
			region = AzureGlobalRegion
		}

		service := pg.Properties.SystemService
		if service == "" && strings.Contains(strings.ToLower(pg.Name), AzureCloudService) {
			service = AzureCloudService
		} else if service == "" {
			service = AzureService
		}
		for _, prefix := range pg.Properties.AddressPrefixes {
			ip, err := ipaddr.NewIPAddressString(prefix).ToAddress()
			if err != nil {
				return nil, nil, fmt.Errorf("invalid IP address %s", prefix)
			}

			if !ip.IsIPv4() {
				continue
			}

			trie.Add(ip.ToIPv4())
			// Azure might allocate the same prefix for multiple usecases. If we
			// find a prefix twice, we prioritise a non-empty SystemService over
			// an empty SystemService and AzureStorageService SystemService over
			// any SystemService.
			//
			// AzureStorage is important because we want to have no false
			// negatives when identifying SMT traffic.
			if detail, ok := remoteIPRanges[ip.ToIPv4().ToKey()]; !ok || (((detail.service == AzureCloudService || detail.service == AzureService || detail.service == "") && pg.Properties.SystemService != "") || (pg.Properties.SystemService == AzureStorageService) || (detail.region == AzureGlobalRegion && region != AzureGlobalRegion && detail.service == service)) {
				// If the prefix hasn't been found, OR
				//
				// If the prefix has been found and its previous service is
				// empty but the current service is not empty, OR
				//
				// If the prefix has been found and its previous service is not
				// empty but the current service is AzureStorage, OR
				//
				// If the prefix has been found and its previous service is the
				// same as the current service but the previous region is global
				// and the current is a specific region
				//
				// Then update prefix info.
				d := remoteIPPrefixDetail{
					cloud:   Azure,
					service: service,
					region:  region,
				}
				d.Normalize()
				remoteIPRanges[ip.ToIPv4().ToKey()] = d
			}
		}
	}

	return remoteIPRanges, trie, nil
}

func getAzureRanges() (azureIPRanges AzureIPRanges, err error) {
	// Unfortunately, Azure doesn't have a solid permalink with their IP ranges.
	// The link contains a date. Azure documentation says it is updated weekly,
	// but we can't trust that. Start with today's date, try to fetch a file
	// that's at most a month old by going back one day at a time, and failing
	// that, fetch a file we know works.
	//
	// It seems the URL prefix before the date is static – e.g. it hasn't
	// changed at least between May 15, 2023 (this StackOverflow answer:
	// https://stackoverflow.com/a/76248208) and August 19, 2024 (when this code
	// was written). TODO: we should store some prefixes as a fallback in a
	// volume.
	for i := 0; i <= 90; i++ {
		date := time.Now().AddDate(0, 0, -1*i).UTC().Format("20060102")
		err = fetchAndParse(fmt.Sprintf("https://download.microsoft.com/download/7/1/D/71D86715-5596-4529-9B13-DA13A5DE5B63/ServiceTags_Public_%v.json", date), &azureIPRanges)
		if err == nil {
			return
		}
	}

	var fallbackURL = "https://download.microsoft.com/download/7/1/D/71D86715-5596-4529-9B13-DA13A5DE5B63/ServiceTags_Public_20240805.json"
	err = fetchAndParse(fallbackURL, &azureIPRanges)
	if err != nil {
		return azureIPRanges, fmt.Errorf("error fetching Azure IP ranges: %w", err)
	}

	return
}

func fetchAndParse(url string, target interface{}) error {
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, target)
	if err != nil {
		return err
	}

	return nil
}

func NormalizeCloudString(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "")
}
