package labeler

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/seancfoley/ipaddress-go/ipaddr"
	"github.com/stretchr/testify/assert"
)

func TestLabelRemote(t *testing.T) {
	t.Parallel()

	remoteLabeler := RemoteLabeler{
		region: "us-west2",
	}
	remoteLabeler.remoteIPPrefixes = map[ipaddr.IPv4AddressKey]remoteIPPrefixDetail{
		ipaddr.NewIPAddressString("1.1.1.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  "us-west2",
			service: "CloudFront",
		},
		ipaddr.NewIPAddressString("2.2.2.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  "ap-southeast1",
			service: "S3",
		},
		ipaddr.NewIPAddressString("3.3.3.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  "ap-southeast2",
			service: "S3",
		},
		ipaddr.NewIPAddressString("9.9.9.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  GoogleGlobalRegion,
			service: GoogleService,
		},
		ipaddr.NewIPAddressString("11.11.11.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  GoogleGlobalRegion,
			service: "Google Cloud",
		},
		ipaddr.NewIPAddressString("12.12.12.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   AWS,
			region:  "usa",
			service: AmazonS3,
		},
		ipaddr.NewIPAddressString("14.14.14.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   GCP,
			region:  "",
			service: "Google Cloud",
		},
		ipaddr.NewIPAddressString("15.15.15.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   AWS,
			region:  AmazonGlobalRegion,
			service: AmazonService,
		},
		ipaddr.NewIPAddressString("16.16.16.0/24").GetAddress().ToIPv4().ToKey(): {
			cloud:   Azure,
			region:  AzureGlobalRegion,
			service: AzureStorageService,
		},
	}
	remoteLabeler.remoteIPPrefixesTrie = ipaddr.NewIPv4AddressTrie()
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("1.1.1.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("2.2.2.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("3.3.3.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("9.9.9.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("11.11.11.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("12.12.12.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("14.14.14.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("15.15.15.0/24").GetAddress().ToIPv4())
	remoteLabeler.remoteIPPrefixesTrie.Add(ipaddr.NewIPAddressString("16.16.16.0/24").GetAddress().ToIPv4())

	tests := []struct {
		name       string
		localCloud Cloud
		endpoint   endpointInfo
		expected   FlowData
		err        error
	}{
		{
			name:       "private IP",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("10.0.0.1"),
			},
			expected: FlowData{
				RemoteCloud:     GCP,
				RemoteRegion:    "us-west2",
				ConnectionClass: IntraVPC,
			},
			err: nil,
		},
		{
			name:       "public IP same region",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("1.1.1.1"),
			},
			expected: FlowData{
				RemoteCloud:        GCP,
				RemoteRegion:       "us-west2",
				RemoteCloudService: "CloudFront",
				ConnectionClass:    IntraRegion,
			},
			err: nil,
		},
		{
			name:       "public IP different region",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("2.2.2.2"),
			},
			expected: FlowData{
				RemoteCloud:        GCP,
				RemoteRegion:       "ap-southeast1",
				RemoteCloudService: "S3",
				ConnectionClass:    InterRegion,
			},
			err: nil,
		},
		{
			name:       "public IP no match",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("8.8.8.8"),
			},
			expected: FlowData{
				RemoteCloud:     "",
				ConnectionClass: PublicInternet,
			},
			err: nil,
		},
		{
			name:       "connecting to a global Google Service",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("9.9.9.9"),
			},
			expected: FlowData{
				RemoteCloud:        GCP,
				ConnectionClass:    IntraRegion,
				RemoteCloudService: GoogleService,
				RemoteRegion:       GoogleGlobalRegion,
			},
			err: nil,
		},
		{
			name:       "connecting to a known IP in a different cloud",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("12.12.12.12"),
			},
			expected: FlowData{
				RemoteCloud:        AWS,
				ConnectionClass:    PublicInternet,
				RemoteCloudService: AmazonS3,
				RemoteRegion:       "usa",
			},
			err: nil,
		},
		{
			name:       "intra-cloud traffic with an empty remote region",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("14.14.14.14"),
			},
			expected: FlowData{
				RemoteCloud:        GCP,
				ConnectionClass:    "",
				RemoteCloudService: "Google Cloud",
				RemoteRegion:       "",
			},
			err: errors.New("found a connection to an undetermined region in the same gcp cloud"),
		},
		{
			name:       "intra-cloud AWS traffic to a global region is intra-region",
			localCloud: AWS,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("15.15.15.15"),
			},
			expected: FlowData{
				RemoteCloud:        AWS,
				ConnectionClass:    IntraRegion,
				RemoteCloudService: AmazonService,
				RemoteRegion:       AmazonGlobalRegion,
			},
			err: nil,
		},
		{
			name:       "intra-cloud GCP traffic to a global region is intra-region",
			localCloud: GCP,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("11.11.11.11"),
			},
			expected: FlowData{
				RemoteCloud:        GCP,
				ConnectionClass:    IntraRegion,
				RemoteCloudService: "Google Cloud",
				RemoteRegion:       GoogleGlobalRegion,
			},
			err: nil,
		},
		{
			name:       "intra-cloud Azure traffic to a global region is intra-region",
			localCloud: Azure,
			endpoint: endpointInfo{
				ip: netip.MustParseAddr("16.16.16.16"),
			},
			expected: FlowData{
				RemoteCloud:        Azure,
				ConnectionClass:    IntraRegion,
				RemoteCloudService: AzureStorageService,
				RemoteRegion:       AzureGlobalRegion,
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FlowData := &FlowData{}
			remoteLabeler.cloud = tt.localCloud
			err := remoteLabeler.labelRemote(&tt.endpoint, FlowData)
			if tt.err == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.err.Error())
			}

			assert.Equal(t, tt.expected.RemoteCloud, FlowData.RemoteCloud)
			assert.Equal(t, tt.expected.RemoteRegion, FlowData.RemoteRegion)
			assert.Equal(t, tt.expected.RemoteCloudService, FlowData.RemoteCloudService)
			assert.Equal(t, tt.expected.ConnectionClass, FlowData.ConnectionClass)
		})
	}
}
