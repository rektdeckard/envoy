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
			name:     "FedEx 12 digits",
			tracking: "123456789012",
			want:     CarrierFedEx,
		},
		{
			name:     "FedEx 15 digits",
			tracking: "123456789012345",
			want:     CarrierFedEx,
		},
		{
			name:     "FedEx 20 digits",
			tracking: "12345678901234567890",
			want:     CarrierFedEx,
		},
		{
			name:     "UPS 18 digits",
			tracking: "123456789012345678",
			want:     CarrierUPS,
		},
		{
			name:     "UPS 1Z",
			tracking: "1Z1234567890123456",
			want:     CarrierUPS,
		},
		{
			name:     "USPS 20 digits",
			tracking: "95001111111111111111",
			want:     CarrierUSPS,
		},
		{
			name:     "USPS 22 digits",
			tracking: "1234567890123456789012",
			want:     CarrierUSPS,
		},
		{
			name:     "DHL 10 digits",
			tracking: "1234567890",
			want:     CarrierDHL,
		},
		{
			name:     "Unknown",
			tracking: "123456789012345678901234567890",
			want:     CarrierUnknown,
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
