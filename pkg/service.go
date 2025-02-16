package service

import (
	"regexp"
	"strings"
)

type Service interface {
	Track(trackingNumbers []string) ([]Parcel, error)
}

type Carrier string

const (
	CarrierFedEx   Carrier = "FedEx"
	CarrierUPS     Carrier = "UPS"
	CarrierUSPS    Carrier = "USPS"
	CarrierDHL     Carrier = "DHL"
	CarrierUnknown Carrier = "Unknown"
)

var trackingPatterns = map[Carrier]*regexp.Regexp{
	CarrierFedEx: regexp.MustCompile(`^\d{12}$|^\d{15}$|^\d{20}$`),
	CarrierUPS:   regexp.MustCompile(`^1Z[a-zA-Z0-9]{16}$|^\d{18}$`),
	CarrierUSPS:  regexp.MustCompile(`^\d{20}$|^\d{22}$`),
	CarrierDHL:   regexp.MustCompile(`^\d{10}$`),
}

var usps20DigitPrefixes = []string{"94", "92", "93", "95"}

// DetectCarrier determines the carrier based on tracking number format
func DetectCarrier(trackingNumber string) Carrier {
	// Normalize the tracking number (remove spaces and dashes)
	trackingNumber = strings.ReplaceAll(trackingNumber, " ", "")
	trackingNumber = strings.ReplaceAll(trackingNumber, "-", "")

	// Check for USPS-specific 20-digit prefixes
	if len(trackingNumber) == 20 {
		for _, prefix := range usps20DigitPrefixes {
			if strings.HasPrefix(trackingNumber, prefix) {
				return CarrierUSPS
			}
		}
		// If it's 20 digits but doesn't match USPS, assume FedEx
		return CarrierFedEx
	}

	// Check patterns for other carriers
	for carrier, pattern := range trackingPatterns {
		if pattern.MatchString(trackingNumber) {
			return carrier
		}
	}

	return CarrierUnknown
}
