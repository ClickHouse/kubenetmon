package labeler

import (
	"net/netip"
	"testing"

	"github.com/seancfoley/ipaddress-go/ipaddr"
	"github.com/stretchr/testify/assert"
)

func TestNewCloud(t *testing.T) {
	_, err := NewCloud("abc")
	assert.Error(t, err)

	_, err = NewCloud("aws")
	assert.NoError(t, err)
}

func TestRefreshRemoteIPs(t *testing.T) {
	t.Run("Test valid AWS prefixes", func(t *testing.T) {
		aws := AWSIPRanges{
			Prefixes: []AWSPrefix{
				{IPPrefixStr: "192.168.0.0/16", Service: "service1", Region: "us-east-1"},
				{IPPrefixStr: "10.0.0.0/8", Service: "service2", Region: "us-west-2"},
				// S3 should override another service.
				{IPPrefixStr: "10.0.0.0/8", Service: "S3", Region: "eu-west-1"},
			},
		}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.NoError(t, err)
		assert.NotNil(t, remoteIPRanges)
		assert.NotNil(t, trie)
		assert.Equal(t, 2, len(remoteIPRanges))

		triePath := trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("192.168.0.1")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   AWS,
			service: "service1",
			region:  "us-east-1",
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])

		triePath = trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("10.0.0.1")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   AWS,
			service: AmazonS3,
			region:  "eu-west-1",
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])
	})

	t.Run("Test invalid AWS prefix", func(t *testing.T) {
		aws := AWSIPRanges{
			Prefixes: []AWSPrefix{
				{IPPrefixStr: "invalid-ip", Service: "service1", Region: "us-east-1"},
			},
		}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.Error(t, err)
		assert.Nil(t, remoteIPRanges)
		assert.Nil(t, trie)
	})

	t.Run("Test valid GCP prefixes", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{
			Prefixes: []GCPPrefix{
				{IPv4PrefixStr: "172.16.0.0/12", Service: "service1", Scope: "europe-north1"},
			},
		}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.NoError(t, err)
		assert.NotNil(t, remoteIPRanges)
		assert.NotNil(t, trie)
		assert.Equal(t, 1, len(remoteIPRanges))

		triePath := trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("172.16.0.1")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   GCP,
			service: "service1",
			region:  "europe-north1",
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])
	})

	t.Run("Test invalid GCP prefix", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{
			Prefixes: []GCPPrefix{
				{IPv4PrefixStr: "invalid-ip", Service: "service1", Scope: "global"},
			},
		}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.Error(t, err)
		assert.Nil(t, remoteIPRanges)
		assert.Nil(t, trie)
	})

	t.Run("Test valid Google prefixes", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{
			Prefixes: []GoogleIPRange{
				{IPv4PrefixStr: "8.8.8.0/24"},
			},
		}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.NoError(t, err)
		assert.NotNil(t, remoteIPRanges)
		assert.NotNil(t, trie)
		assert.Equal(t, 1, len(remoteIPRanges))

		triePath := trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("8.8.8.8")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   GCP,
			service: GoogleService,
			region:  GoogleGlobalRegion,
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])
	})

	t.Run("Test invalid Google prefix", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{
			Prefixes: []GoogleIPRange{
				{IPv4PrefixStr: "invalid-ip"},
			},
		}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.Error(t, err)
		assert.Nil(t, remoteIPRanges)
		assert.Nil(t, trie)
	})

	t.Run("Test valid Azure prefixes", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{
			PrefixGroups: []AzurePrefixGroup{
				{
					Name: "ActionGroup.GermanyWestCentral",
					Properties: AzurePrefixGroupProperties{
						Region:          "india",
						SystemService:   "service1",
						AddressPrefixes: []string{"1.1.1.1/32"},
					},
				},
				{
					Name: "ActionGroup.GermanyWestCentral",
					Properties: AzurePrefixGroupProperties{
						Region: "germanywestcentral",
						// AzureStorage service should take priority over
						// another service for the same prefix.
						SystemService:   AzureStorageService,
						AddressPrefixes: []string{"1.1.1.1/32"},
					},
				},
				{
					Name: "ActionGroup.WestUS3",
					Properties: AzurePrefixGroupProperties{
						Region:          "",
						SystemService:   "",
						AddressPrefixes: []string{"2.2.2.2/32"},
					},
				},
				{
					Name: "ActionGroup.WestUS3",
					Properties: AzurePrefixGroupProperties{
						Region: "",
						// Non-empty service should take priority over an empty
						// service for the same prefix.
						SystemService:   "non-empty",
						AddressPrefixes: []string{"2.2.2.2/32"},
					},
				},
			},
		}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.NoError(t, err)
		assert.NotNil(t, remoteIPRanges)
		assert.NotNil(t, trie)
		assert.Equal(t, 2, len(remoteIPRanges))

		triePath := trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("1.1.1.1")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   Azure,
			service: AzureStorageService,
			region:  "germanywestcentral",
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])

		triePath = trie.ElementsContaining(ipaddr.NewIPAddressFromNetNetIPAddr(netip.MustParseAddr("2.2.2.2")).ToIPv4())
		assert.Greater(t, triePath.Count(), 0)
		assert.Equal(t, remoteIPPrefixDetail{
			cloud:   Azure,
			service: "non-empty",
			region:  AzureGlobalRegion,
		}, remoteIPRanges[triePath.LongestPrefixMatch().GetKey().ToKey()])
	})

	t.Run("Test invalid Azure prefixes", func(t *testing.T) {
		aws := AWSIPRanges{}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{
			PrefixGroups: []AzurePrefixGroup{
				{
					Name: "ActionGroupGermanyWestCentral",
					Properties: AzurePrefixGroupProperties{
						Region:          "GermanyWestCentral",
						AddressPrefixes: []string{"aaaaa"},
					},
				},
			},
		}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.Error(t, err)
		assert.Nil(t, remoteIPRanges)
		assert.Nil(t, trie)
	})

	t.Run("Test AWS service priority", func(t *testing.T) {
		awsServicePriorities = map[string]int{
			"service1": 1,
			"service2": 2,
		}

		aws := AWSIPRanges{
			Prefixes: []AWSPrefix{
				{IPPrefixStr: "192.168.0.0/16", Service: "service2", Region: "us-west-2"},
				{IPPrefixStr: "192.168.0.0/16", Service: "service1", Region: "us-east-1"},
			},
		}
		gcp := GCPIPRanges{}
		google := GoogleIPRanges{}
		azure := AzureIPRanges{}

		remoteIPRanges, trie, err := refreshRemoteIPs(aws, gcp, google, azure)
		assert.NoError(t, err)
		assert.NotNil(t, remoteIPRanges)
		assert.NotNil(t, trie)

		assert.Equal(t, 1, len(remoteIPRanges))
		ip, err := ipaddr.NewIPAddressString("192.168.0.0/16").ToAddress()
		assert.NoError(t, err)
		assert.NotNil(t, ip)
		detail := remoteIPRanges[ip.ToIPv4().ToKey()]
		assert.Equal(t, "service1", detail.service)
		assert.Equal(t, "us-east-1", detail.region)
	})
}

func TestNormalizeCloudString(t *testing.T) {
	assert.Equal(t, "germanywestcentral", NormalizeCloudString("Germany West Central"))
	assert.Equal(t, "us-east1", NormalizeCloudString("us-east1"))
}
