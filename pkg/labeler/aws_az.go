package labeler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"

	"github.com/seancfoley/ipaddress-go/ipaddr"
)

type AWSAZInfoProvider struct {
	// subnetPrefixes is a map from subnet ip prefix to its az detail
	// subnetPrefixesTrie is for fast lookup
	// This is only needed in AWS, because only AWS charge cross AZ traffic and possible to have per AZ subnet assignment.
	subnetPrefixes     map[ipaddr.IPv4AddressKey]azDetail
	subnetPrefixesTrie *ipaddr.IPv4AddressTrie
}

// struct for the subnet details
type SubnetDetail struct {
	AZ   string `json:"az"`
	Cidr string `json:"cidr"`
}

// struct for the regions
type Region struct {
	Region  string                    `json:"region"`
	Subnets []map[string]SubnetDetail `json:"subnets"`
}

func NewAWSAZInfoProvider() (*AWSAZInfoProvider, error) {
	// If this is AWS cloud, try to read the AZ cidr map from env variable.
	// The map is encoded in a json string, which likes the following
	/*
		[
			{
				"region": "us-west-2",
				"subnets": [
					{
						"private-a": {
							"az": "usw2-az2",
							"cidr": "10.1.0.0/22"
						},
						"private-b": {
							"az": "usw2-az1",
							"cidr": "10.1.4.0/22"
						}
					},
					{
						"private-a": {
							"az": "a",
							"cidr": "10.20.0.0/22"
						}
					}
				]
			},
			{
				"region": "eu-central-1",
				"subnets": [
					{
						"private-a": {
							"az": "a",
							"cidr": "10.30.0.0/22"
						}
					}
				]
			}
		]
	*/

	azMapEnv, ok := os.LookupEnv("AWS_AZ_CIDR_MAP")
	if !ok {
		return nil, errors.New("AWS_AZ_CIDR_MAP env var should not be empty")
	}

	var regions []Region
	err := json.Unmarshal([]byte(azMapEnv), &regions)
	if err != nil {
		return nil, fmt.Errorf("AWS_AZ_CIDR_MAP is malformed, cannot unmarshal: %v", err)
	}
	awsAZInfoProvider := &AWSAZInfoProvider{
		subnetPrefixes:     make(map[ipaddr.IPv4AddressKey]azDetail),
		subnetPrefixesTrie: ipaddr.NewIPv4AddressTrie(),
	}

	for _, r := range regions {
		for _, subnet := range r.Subnets {
			for _, detail := range subnet {
				ip, err := ipaddr.NewIPAddressString(detail.Cidr).ToAddress()
				if err != nil {
					return nil, fmt.Errorf("invalid address %s", detail.Cidr)
				}
				addr := ip.ToIPv4()
				awsAZInfoProvider.subnetPrefixesTrie.Add(addr)
				awsAZInfoProvider.subnetPrefixes[addr.ToKey()] = azDetail{
					azID:   detail.AZ,
					region: r.Region,
				}
			}
		}
	}
	return awsAZInfoProvider, nil
}

func (l *AWSAZInfoProvider) findAZDetail(ip netip.Addr) azDetail {
	addr := ipaddr.NewIPAddressFromNetNetIPAddr(ip).ToIPv4()
	triePath := l.subnetPrefixesTrie.ElementsContaining(addr)
	if triePath.Count() > 0 {
		return l.subnetPrefixes[triePath.LongestPrefixMatch().GetKey().ToKey()]
	}

	return azDetail{}
}
