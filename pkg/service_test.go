package envoy

import (
	"testing"
)

func TestDetectCarrier(t *testing.T) {
	tests := []struct {
		name     string
		tracking string
		want     Carrier
	}{
		{
			name:     "USPS GS1-128 (91)",
			tracking: "9102001234567890123456",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS GS1-128 (92)",
			tracking: "9261290339741308689554",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS GS1-128 (93)",
			tracking: "9302001234567890123456",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS First-Class",
			tracking: "9400123456789012345678",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS Express Int'l",
			tracking: "EC123456789US",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS Registered Mail",
			tracking: "9208123456789012345678",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS 20 digits",
			tracking: "95001111111111111111",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS realworld example",
			tracking: "92001903104186015180053869",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS realworld example 2",
			tracking: "92184903716531000000100565",
			want:     CarrierUSPS,
		},
		{
			name:     "UPS 1Z",
			tracking: "1Z1234567890123456",
			want:     CarrierUPS,
		},
		{
			name:     "UPS Mail Innovations",
			tracking: "MI1234567890123456",
			want:     CarrierUPS,
		},
		{
			name:     "UPS Freight",
			tracking: "H9999999999",
			want:     CarrierUPS,
		},
		{
			name:     "UPS alternative format",
			tracking: "T1234567890",
			want:     CarrierUPS,
		},
		{
			name:     "FedEx Express (12 digits)",
			tracking: "123456789012",
			want:     CarrierFedEx,
		},
		{
			name:     "FedEx Ground (96...)",
			tracking: "9612345678901234567890",
			want:     CarrierFedEx,
		},
		{
			name:     "FedEx Ground (15 digits)",
			tracking: "999999999999999",
			want:     CarrierFedEx,
		},
		{
			name:     "FedEx door tag",
			tracking: "DT123456789012",
			want:     CarrierFedEx,
		},
		{
			name:     "DHL Express (10 digits)",
			tracking: "1234567890",
			want:     CarrierDHL,
		},
		{
			name:     "DHL Express (JDD...)",
			tracking: "JJD1234567890",
			want:     CarrierDHL,
		},
		{
			name:     "DHL Express (5...)",
			tracking: "5123456789",
			want:     CarrierDHL,
		},
		{
			name:     "DHL German",
			tracking: "JJD123456789012345678",
			want:     CarrierDHL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectCarrier(tt.tracking); got != tt.want {
				t.Errorf("DetectCarrier() = %v, want %v", got, tt.want)
			}
		})
	}
}
