package envoy

import (
	"fmt"
	"regexp"
	"strings"

	"dev.freespoke.com/go-package-tracking"
)

type Service interface {
	Track(trackingNumbers []string) ([]*Parcel, error)
	Reauthenticate() error
}

type Carrier string

const (
	CarrierFedEx     Carrier = "FedEx"
	CarrierUPS       Carrier = "UPS"
	CarrierUSPS      Carrier = "USPS"
	CarrierDHL       Carrier = "DHL"
	CarrierAmazon    Carrier = "Amazon"
	CarrierOnTrac    Carrier = "OnTrac"
	CarrierLaserShip Carrier = "LaserShip"
	CarrierUnknown   Carrier = "Unknown"
)

// DetectCarrier determines the carrier based on tracking number format
func DetectCarrier(trackingNumber string) Carrier {
	// Remove any spaces, hyphens, or other common separators
	trackingNumber = strings.ReplaceAll(trackingNumber, " ", "")
	trackingNumber = strings.ReplaceAll(trackingNumber, "-", "")
	trackingNumber = strings.ToUpper(trackingNumber)

	p, _ := parcel.Track(trackingNumber)
	fmt.Printf("p: %+v\n", p)

	// First try to determine carrier by distinctive patterns
	if isDHL(trackingNumber) {
		return CarrierDHL
	}

	if isUPS(trackingNumber) {
		return CarrierUPS
	}

	if isFedEx(trackingNumber) {
		return CarrierFedEx
	}

	// USPS check comes last as it has many formats, some similar to other carriers
	if _, isUSPS := isUSPS(trackingNumber); isUSPS {
		return CarrierUSPS
	}

	return CarrierUnknown
}

// isDHL checks if the tracking number is a valid DHL tracking number
func isDHL(trackingNumber string) bool {
	patterns := []string{
		// Standard DHL Express: 10 digits
		`^\d{10}$`,

		// DHL Express with JJD/JJD01/JJD00 prefix: 10 or 11 digits
		`^JJD0?1?\d{10,11}$`,

		// DHL Express starting with 1 and 10 digits
		`^1\d{9}$`,

		// Standard DHL eCommerce: Several fixed formats
		`^\d{4}[- ]?\d{4}[- ]?\d{2}$`,
		`^[A-Z]{3}\d{7}$`,
		`^[A-Z]{5}\d{10}$`,
		`^420\d{27}$`,

		// German DHL: always 20 chars; either all numbers or starts with "JJD" followed by 18 digits
		`^(JJD\d{18}|\d{20})$`,

		// International DHL: always numeric and 10 or 11 digits
		`^\d{10,11}$`,
	}

	// DHL patterns that could overlap with other carriers are further disambiguated
	overlappingPatterns := map[string]bool{
		// 10-digit DHL that overlaps with USPS money orders
		// DHL format always starts with numbers >= 5
		`^[5-9]\d{9}$`: true,
	}

	// Check non-overlapping patterns first
	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched {
			// For 10-11 digit patterns, ensure it doesn't match UPS or FedEx specific patterns
			if len(trackingNumber) == 10 || len(trackingNumber) == 11 {
				if strings.HasPrefix(trackingNumber, "1Z") {
					return false // This is likely a UPS tracking number
				}
			}
			return true
		}
	}

	// Check potentially overlapping patterns
	for pattern := range overlappingPatterns {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched {
			// DHL 10-digit tracking usually starts with 5-9
			firstDigit := int(trackingNumber[0] - '0')
			if firstDigit >= 5 {
				return true
			}
		}
	}

	return false
}

// isUPS checks if the tracking number is a valid UPS tracking number
func isUPS(trackingNumber string) bool {
	patterns := []string{
		// UPS tracking number format: 1Z + 6 alphanumeric + 2 digits + 8 digits
		`^1Z[A-Z0-9]{6}\d{2}\d{8}$`,

		// UPS Mail Innovations: starts with MI, YW, or UP prefix followed by digits
		`^(MI|YW|UP)\d{15,22}$`,

		// UPS Freight: starts with H followed by 9 or 10 digits
		`^H\d{9,10}$`,

		// UPS alternative format (rare but exists): 9 digits
		`^T\d{10}$`,
		`^\d{9}$`,

		// UPS SurePost: Start with 92 but have specific handling and can often be verified by character count
		`^92\d{17,20}$`,

		// UPS Next Day Air & 2nd Day Air
		`^[0-9]{12}$`,

		// UPS Innovations (USPS delivery for Last Mile)
		`^[0-9]{18}$`,
	}

	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched {
			// Special handling for the 92-prefix format
			// UPS SurePost deliveries vs USPS
			if strings.HasPrefix(trackingNumber, "92") {
				// UPS SurePost typically has 20 digits total, but need more logic for certainty
				// This is a simplified check, more sophisticated checks would consider check digits
				return len(trackingNumber) == 20
			}

			// 1Z is a distinctive UPS prefix
			if strings.HasPrefix(trackingNumber, "1Z") {
				return true
			}

			// For 9-digit formats, verify it's not a USPS format
			if len(trackingNumber) == 9 && regexp.MustCompile(`^\d{9}$`).MatchString(trackingNumber) {
				// This would need additional logic to be certain
				return true
			}

			return true
		}
	}

	return false
}

// isFedEx checks if the tracking number is a valid FedEx tracking number
func isFedEx(trackingNumber string) bool {
	patterns := []string{
		// FedEx Express (air): 12 digits
		`^\d{12}$`,

		// FedEx Ground: 15 digits, starts with 96 or 98
		`^(96|98)\d{13}$`,

		// FedEx SmartPost: 20 digits
		// Can start with 92 (shared with USPS) but specific length
		`^92\d{18}$`,

		// FedEx Express (international): 12 digits
		`^\d{12}$`,

		// FedEx Ground (96...)
		`^96\d{20}$`,

		// FedEx Ground Home Delivery
		`^9\d{11}$`,

		// FedEx Ground 15-digit barcode format (all numeric)
		`^\d{15}$`,

		// FedEx 2D tracking codes - typically 14 alpha/numeric
		`^[A-Z0-9]{14}$`,

		// FedEx Ground SSCC-18 barcode format
		`^\d{18}$`,

		// FedEx door tag number
		`^DT\d{12}$`,
	}

	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched {
			// For 12-digit format (which could be shared with UPS),
			// we need additional check logic
			if len(trackingNumber) == 12 && regexp.MustCompile(`^\d{12}$`).MatchString(trackingNumber) {
				// Certain FedEx patterns have check digit validation
				// (simplified example - real validation would involve more complex math)
				return true
			}

			// For SSCC-18 format (shared with other carriers), verify it's FedEx
			if len(trackingNumber) == 18 && regexp.MustCompile(`^\d{18}$`).MatchString(trackingNumber) {
				// Would need additional logic to be certain
				return true
			}

			// 92-prefix formats with length 20 can be FedEx SmartPost
			if strings.HasPrefix(trackingNumber, "92") && len(trackingNumber) == 20 {
				// This would need additional verification for certainty
				return true
			}

			// 96/98 prefixes are distinctive to FedEx Ground
			if strings.HasPrefix(trackingNumber, "96") || strings.HasPrefix(trackingNumber, "98") {
				return true
			}

			// DT prefix is distinctive to FedEx door tags
			if strings.HasPrefix(trackingNumber, "DT") {
				return true
			}

			return true
		}
	}

	return false
}

// isUSPS checks if the tracking number is a valid USPS tracking number
// Returns the format name and a boolean indicating validity
func isUSPS(trackingNumber string) (string, bool) {
	// Define patterns for different USPS tracking number formats with their format names
	formats := map[string]string{
		// GS1-128 Formats with 91 prefix (USPS specific)
		`^91\d{18}$`: "USPS GS1-128 (91)",

		// For 92, 93, 94 prefixes, we need to be selective since they're shared with other carriers
		// 92-prefix that is distinctly USPS and not UPS/FedEx
		`^92[1-7]\d{17}$`: "USPS GS1-128 (92)",
		`^93\d{18}$`:      "USPS GS1-128 (93)",
		`^94\d{18}$`:      "USPS GS1-128 (94)",

		// 22-digit format (91 prefix - USPS specific)
		`^91\d{20}$`: "USPS 22-digit",

		// 30-digit format with ZIP Code (USPS specific)
		`^420\d{5}91\d{18}$`: "USPS ZIP+GS1",

		// Format with 420 (ZIP) + S.T.I. - USPS specific
		`^420\d{5}[0-9]{2}\d{12}$`: "USPS ZIP+STI",

		// 34-digit USPS Electronic Shipping Info
		`^420\d{5}91\d{27}$`: "USPS Electronic Shipping",

		// Legacy and Special USPS-specific Formats
		`^[A-Z]{2}\d{9}US$`: "USPS International",

		// 13-character domestic format (USPS-specific)
		`^\d{4}\d{9}$`: "USPS 13-char Domestic",

		// 20-character format (USPS-specific international)
		`^[A-Z]{2}\d{9}[A-Z0-9]{9}$`: "USPS 20-char International",

		// Priority Mail Express (USPS-specific)
		`^E[A-Z]\d{9}[A-Z]$`: "USPS Priority Express A",
		`^E[A-Z]\d{9}$`:      "USPS Priority Express B",

		// Certified Mail (USPS-specific)
		`^9407\d{16}$`: "USPS Certified Mail",

		// Registered Mail (USPS-specific)
		`^9208\d{16}$`: "USPS Registered Mail",

		// Express Mail International (USPS-specific)
		`^EC\d{9}[A-Z]{2}$`: "USPS Express Int'l",

		// Money Order (USPS-specific)
		`^[1-4]\d{9,10}$`: "USPS Money Order",

		// Military Mail (USPS-specific)
		`^[A-Z]{2}\d{9}$`: "USPS Military Mail",

		// International inbound (USPS-specific)
		`^[A-Z]{2}\d{9}[A-Z]{2}$`: "USPS Int'l Inbound",

		// Signature Confirmation (USPS-specific)
		`^9202\d{16}$`: "USPS Signature Conf A",
		`^9202\d{20}$`: "USPS Signature Conf B",

		// Standard post package (USPS-specific)
		`^03\d{18}$`: "USPS Standard Post",

		// COD tracking (USPS-specific)
		`^9303\d{16}$`: "USPS COD",

		// Insured mail (USPS-specific)
		`^92[0-9][0-9]\d{16}$`: "USPS Insured Mail",

		// First-Class Package (USPS-specific)
		`^9400\d{16}$`: "USPS First-Class",

		// Return Receipt (USPS-specific)
		`^9590\d{16}$`: "USPS Return Receipt",

		// Not sure??
		`^92\d{20}$`:    "USPS Unknown",
		`^93\d{18,20}$`: "USPS Unknown",
		`^94\d{18,20}$`: "USPS Unknown",
		`^95\d{18,20}$`: "USPS Unknown",
	}

	// Special case formats that need additional checks to avoid overlapping with other carriers
	specialCases := map[string]func(string) bool{
		// 13-digit all numeric (might overlap with UPS and FedEx)
		`^\d{13}$`: func(tn string) bool {
			// USPS 13-digit typically starts with specific digits
			// Additional check needed to disambiguate from other carriers
			return !strings.HasPrefix(tn, "1Z")
		},

		// IMpb (24-31 chars) needs additional verification due to overlap
		`^[A-Z0-9]{24,31}$`: func(tn string) bool {
			// Intelligent Mail Package Barcode has specific structure
			// Simplified check - real validation would be more complex
			return strings.HasPrefix(tn, "9") && !strings.HasPrefix(tn, "96") && !strings.HasPrefix(tn, "98")
		},
	}

	// Check standard formats first
	for pattern, formatName := range formats {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched {
			// For 92-prefix, verify it's not a UPS SurePost or FedEx SmartPost
			if strings.HasPrefix(trackingNumber, "92") {
				// Different lengths can indicate different carriers
				switch len(trackingNumber) {
				case 20:
					// Need more sophisticated check for complete certainty
					// This is a simplified check
					if strings.HasPrefix(trackingNumber, "9205") ||
						strings.HasPrefix(trackingNumber, "9207") ||
						strings.HasPrefix(trackingNumber, "9208") ||
						strings.HasPrefix(trackingNumber, "9210") {
						return formatName, true
					}
				case 22:
					return formatName, true
				}
			} else {
				return formatName, true
			}
		}
	}

	// Check special cases that need additional verification
	for pattern, verifyFunc := range specialCases {
		matched, _ := regexp.MatchString(pattern, trackingNumber)
		if matched && verifyFunc(trackingNumber) {
			return "USPS Special Format", true
		}
	}

	return "", false
}
