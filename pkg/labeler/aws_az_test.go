package labeler

import (
	"net/netip"
	"os"
	"testing"
)

func TestFindAZDetail(t *testing.T) {
	t.Parallel()

	// Set up the environment variable for testing
	azMap := `
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
	`

	os.Setenv("AWS_AZ_CIDR_MAP", azMap)
	defer os.Unsetenv("AWS_AZ_CIDR_MAP")

	provider, err := NewAWSAZInfoProvider()
	if err != nil {
		t.Fatalf("unexpected error initializing AWSAZInfoProvider: %v", err)
	}

	tests := []struct {
		ip       string
		expected azDetail
	}{
		{
			ip: "10.1.1.1",
			expected: azDetail{
				azID:   "usw2-az2",
				region: "us-west-2",
			},
		},
		{
			ip: "10.1.4.1",
			expected: azDetail{
				azID:   "usw2-az1",
				region: "us-west-2",
			},
		},
		{
			ip: "10.30.0.1",
			expected: azDetail{
				azID:   "a",
				region: "eu-central-1",
			},
		},
		{
			ip:       "192.168.1.1",
			expected: azDetail{}, // No matching subnet
		},
	}

	for _, test := range tests {
		ipAddr, err := netip.ParseAddr(test.ip)
		if err != nil {
			t.Fatalf("failed to parse IP address %s: %v", test.ip, err)
		}

		result := provider.findAZDetail(ipAddr)
		if result != test.expected {
			t.Errorf("for IP %s, expected %v, got %v", test.ip, test.expected, result)
		}
	}
}
