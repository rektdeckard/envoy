package ups

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rektdeckard/envoy/pkg"
)

var (
	BaseURL, _ = url.Parse("https://onlinetools.ups.com")
)

type UPSService struct {
	Client    *http.Client
	APIKey    string
	APISecret string
	Token     *Token
}

// Enforce that UPSService implements the Service interface
var _ envoy.Service = &UPSService{}

func NewUPSService(client *http.Client, apiKey, apiSecret string) *UPSService {
	return &UPSService{
		Client:    client,
		APIKey:    apiKey,
		APISecret: apiSecret,
	}
}

func (s *UPSService) Reauthenticate() error {
	res := GetAccessToken(s.Client, s.APIKey, s.APISecret, nil, nil)

	if res.Error != "" {
		return fmt.Errorf("error getting access token: %s", res.Error)
	}

	expiresIn, err := strconv.Atoi(res.Response.ExpiresIn)
	if err != nil {
		return err
	}
	s.Token = &Token{
		value:      res.Response.AccessToken,
		expiration: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	return nil
}

func (s *UPSService) Track(trackingNumbers []string) ([]*envoy.Parcel, error) {
	const endpoint = "/api/track/v1/details/"

	if s.Token == nil || !s.Token.isValid() {
		if err := s.Reauthenticate(); err != nil {
			return nil, err
		}
	}

	params := url.Values{
		"locale":           []string{"en_US"},
		"returnSignature":  []string{"false"},
		"returnMilestones": []string{"false"},
		"returnPOD":        []string{"false"},
	}
	headers := http.Header{
		"Authorization":  []string{"Bearer " + s.Token.value},
		"TransId":        []string{"1ZW701150378674373"},
		"TransactionSrc": []string{"envoy"},
	}

	var parcels []*envoy.Parcel
	// wg := sync.WaitGroup{}

	for _, trackingNumber := range trackingNumbers {
		url := BaseURL.ResolveReference(&url.URL{Path: endpoint + trackingNumber})
		url.RawQuery = params.Encode()

		req, err := http.NewRequest(http.MethodGet, url.String(), nil)
		if err != nil {
			return nil, err
		}

		req.Header = headers

		res, err := s.Client.Do(req)
		if err != nil {
			return nil, err
		}

		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		// fmt.Println(string(body))

		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
		}

		var trackingRes response
		if err := json.Unmarshal(body, &trackingRes); err != nil {
			return nil, err
		}
		// d, _ := json.MarshalIndent(trackingRes, "", "  ")
		// fmt.Println(string(d))

		for _, shipment := range trackingRes.TrackResponse.Shipment {
			for _, p := range shipment.Package {
				// TODO: figure out a default name for the parcel
				name := p.TrackingNumber
				parcel := envoy.NewParcel(
					name,
					envoy.CarrierUPS,
					p.TrackingNumber,
					fmt.Sprintf("https://www.ups.com/track?tracknum=%s", p.TrackingNumber),
				)
				parcel.Data = &envoy.ParcelData{}

				for _, dd := range p.DeliveryDate {
					if dd.Type != DeliveryDateTypeScheduled && dd.Type != DeliveryDateTypeRescheduled {
						continue
					}
					d, err := time.Parse("20060102", dd.Date)
					if err != nil {
						log.Fatalf("error parsing delivery date: %v", err)
						continue
					}
					if parcel.Data.DeliveryProjection != nil && d.After(*parcel.Data.DeliveryProjection) {
						parcel.Data.DeliveryProjection = &d
					}
				}

				var lastEvent *Activity
				for _, a := range p.Activity {
					if lastEvent == nil || a.Date > lastEvent.Date {
						lastEvent = a
					}
					if a.Status.Type == "D" || a.Status.Code == "FS" {
						parcel.Data.Delivered = true
					}
					parcel.Data.Events = append(parcel.Data.Events, envoy.ParcelEvent{
						Timestamp:   a.Timestamp(),
						Description: a.Status.Description,
						Location:    a.Location.Address.String(),
						Type:        a.Status.ParcelEventType(),
					})
				}

				parcels = append(parcels, parcel)
			}
		}
	}

	return parcels, nil
}

type Token struct {
	value      string
	expiration time.Time
}

func (t *Token) isValid() bool {
	return t.expiration.After(time.Now())
}

type response struct {
	TrackResponse struct {
		Shipment []*Shipment `json:"shipment"`
	} `json:"trackResponse"`
}

type Shipment struct {
	InquiryNumber string     `json:"inquiryNumber"`
	Package       []*Package `json:"package"`
}

type Package struct {
	TrackingNumber          string                     `json:"trackingNumber"`
	AlternateTrackingNumber []*AlternateTrackingNumber `json:"alternateTrackingNumber"`
	Activity                []*Activity                `json:"activity"`
	Milestones              []*Milestone               `json:"milestones"`
	CurrentStatus           *Status                    `json:"currentStatus"`
	DeliveryDate            []*DeliveryDate            `json:"deliveryDate"`
	DeliveryTime            *DeliveryTime              `json:"deliveryTime"`
	// Container with all information related to the delivery of the package.
	// Populated only when the package is delivered.
	DeliveryInformation *DeliveryInformation `json:"deliveryInformation"`
	Dimension           Dimension            `json:"dimension"`
	PackageAddress      []*PackageAddress    `json:"packageAddress"`
	// The total number of packages in the shipment.
	// Note that this number may be greater than the number of returned packages in the
	// response. In such cases subsequent calls are needed to get additional packages.
	PackageCount           int32                   `json:"packageCount"`
	AccessPointInformation *AccessPointInformation `json:"accessPointInformation"`
	// The list of additional attributes that may be associated with the package.
	// Presence of any element indicates the package has that attribute.
	AdditionalAttributes []string `json:"additionalAttributes"`
	// The list of additional services that may be associated with the package.
	// Presence of any element indicates that the package has that service.
	AdditionalServices []string `json:"additionalServices"`
	IsSmartPackage     bool     `json:"isSmartPackage"`
}

type PackageAddress struct {
	Address *Address `json:"address"`
	// The specific name of an individual associated with the address segment.
	AttentionName string `json:"attentionName"`
	// Ship-to name.
	Name string `json:"name"`
	// The type of address.
	Type string `json:"type"`
}

type AlternateTrackingNumber struct {
	Number string `json:"number"`
	// The type of alternate number. Non-typed numbers are typically UPS tracking numbers.
	Type string `json:"type"`
}

// The container that has all the information related to the access point where the package is destined for/delivered to.
type AccessPointInformation struct {
	// Format: "YYYYMMDD"
	PickupByDate string `json:"pickupByDate"`
}

type DeliveryDate struct {
	Type DeliveryDateType `json:"type"`
	// The date of this delivery detail. Format: YYYYMMDD
	Date string `json:"date"`
}

type DeliveryDateType string

const (
	DeliveryDateTypeScheduled   DeliveryDateType = "SDD"
	DeliveryDateTypeRescheduled DeliveryDateType = "RDD"
	DeliveryDateTypeActual      DeliveryDateType = "DEL"
)

type DeliveryTime struct {
	Type    string `json:"type"`
	EndTime string `json:"endTime"` // "HHMMSS"
}

type DeliveryInformation struct {
	Location      string         `json:"location"`
	ReceivedBy    string         `json:"receivedBy"`
	Signature     *Signature     `json:"signature"`
	DeliveryPhoto *DeliveryPhoto `json:"deliveryPhoto"`
	POD           *POD           `json:"pod"`
}

type Signature struct {
	Image string `json:"image"`
}

type DeliveryPhoto struct {
	Photo string `json:"photo"`
	// The indication if the photo is a capture or not.
	PhotoCaptureInd      PhotoCaputureIndicator `json:"photoCaptureIndicator"`
	PhotoDispositionCode PhotoDispositionCode   `json:"photoDispositionCode"`
	// The indication if the country does not use postal code. Valid values:
	// 'true' this country does not use postal code.
	// 'false' this country uses postal code.
	IsNonPostalCodeCountry bool `json:"isNonPostalCodeCountry"`
}

type PhotoCaputureIndicator bool

func (p PhotoCaputureIndicator) UnmarshallJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "Y":
		p = true
	default:
		p = false
	}
	return nil
}

type PhotoDispositionCode string

const (
	PhotoDispositionCodeViewable    PhotoDispositionCode = "V"
	PhotoDispositionCodeNotViewable PhotoDispositionCode = "N"
	PhotoDispositionCodeUnstored    PhotoDispositionCode = "U"
)

// Container which contains Proof of Delivery.
type POD struct {
	// The base64 encoded string representation of the Delivery Proof. Note: This is considered sensitive data and may only be returned for a user that has rights to the package.
	Content string `json:"content"`
}

type Dimension struct {
	Height          string `json:"height"`
	Length          string `json:"length"`
	Width           string `json:"width"`
	UnitOfDimension string `json:"unitOfDimension"`
}

type Activity struct {
	Location *Location `json:"location"`
	Status   *Status   `json:"status"`
	// The date of the activity. Format: YYYYMMDD
	Date string `json:"date"`
	// The time of the activity. Format: HHMMSS (24 hr)
	Time string `json:"time"` // "HHMMSS"
	// The GMT date of the activity. Format: YYYYMMDD
	GMTDate string `json:"gmtDate"`
	// The GMT time of the activity. Format: HH:MM:SS (24 hr)
	GMTTime string `json:"gmtTime"`
	// The GMT offset of the activity. Format: -HH:MM
	GMTOffset string `json:"gmtOffset"`
}

func (a *Activity) Timestamp() time.Time {
	t, _ := time.Parse("20060102150405", a.Date+a.Time)
	return t
}

type Milestone struct {
	Code string `json:"code"`
	// The milestone category. This will be present only when a milestone is in a COMPLETE state.
	Category string `json:"category"`
	// The indication if the milestone represents the current state of the package.
	// Valid values: 'true' this milestone is the current state of the package.
	// 'false' this milestone is not current.
	Current bool `json:"current"`
	// The milestone description. Note: this is not translated at this time and is returned in US English.
	Description string `json:"description"`
	// The 0-based index of the activity that triggered this milestone. This will be returned only when a milestone is in a COMPLETE state. For example the most recent activity on the response is index 0.
	LinkedActivity string `json:"linkedActivity"`
	// The milestone state. Valid values: 'This milestone has already occurred'/'This milestone has not yet been completed'.
	State MilestoneState `json:"state"`
	// The sub-milestone container containing information on a child milestone. Will be present only if a child milestone exists.
	SubMilestone struct {
		Category string `json:"category"`
	} `json:"subMilestone"`
}

type MilestoneState string

const (
	MilestoneStateComplete   MilestoneState = "This milestone has already occurred"
	MilestoneStateIncomplete MilestoneState = "This milestone has not yet been completed"
)

type Location struct {
	Address *Address `json:"address"`
	// Site Location Indicator Code (SLIC)
	SLIC string `json:"slic"`
}

type Address struct {
	AddressLine1  string `json:"addressLine1"`
	AddressLine2  string `json:"addressLine2"`
	AddressLine3  string `json:"addressLine3"`
	City          string `json:"city"`
	StateProvince string `json:"stateProvince"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	PostalCode    string `json:"postalCode"`
}

func (a *Address) String() string {
	sb := strings.Builder{}
	if a.City != "" {
		sb.WriteString(a.City)
		if a.StateProvince != "" {
			sb.WriteString(", ")
		}
	}
	sb.WriteString(a.StateProvince)
	if a.PostalCode != "" {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(a.PostalCode)
	}
	if a.CountryCode != "US" {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(a.CountryCode)
	}
	if sb.Len() == 0 {
		return "—"
	}
	return strings.ToUpper(sb.String())
}

type Status struct {
	Type string `json:"type"`
	// Status description. Note that this field will be translated based on the locale provided in the request.
	Description string `json:"description"`
	Code        string `json:"code"`
	// The activity package detail status code see API Codes for possible values.
	StatusCode string `json:"statusCode"`
	// The current status in simplified text. This is a supplementary description providing
	// additional information on the status of the package. Note that this field will be
	// translated based on the locale provided in the request.
	SimplifiedTextDescription string `json:"simplifiedTextDescription"`
}

func (s *Status) ParcelEventType() envoy.ParcelEventType {
	switch s.Code {
	case "MP":
		return envoy.ParcelEventTypeOrderConfirmed
	case "OR", "AR":
		return envoy.ParcelEventTypeArrived
	case "YP":
		return envoy.ParcelEventTypeProcessing
	case "DP":
		return envoy.ParcelEventTypeDeparted
	case "OF":
		return envoy.ParcelEventTypeOnVehicle
	case "OT":
		return envoy.ParcelEventTypeOutForDelivery
	case "FS":
		return envoy.ParcelEventTypeDelivered
	default:
		switch s.StatusCode {
		// https://developer.ups.com/api/reference/tracking/appendix?loc=en_US
		case "00", "09", "12", "2D", "2J", "32", "41", "42", "44":
			return envoy.ParcelEventTypeDelayed
		case "1N", "1Z", "2C", "2Q":
			return envoy.ParcelEventTypeAwaitingCustomerPickup
		case "28":
			return envoy.ParcelEventTypeParcelHeld
		case "2K":
			return envoy.ParcelEventTypeOutForDelivery
		case "2W", "3F", "3G", "3H":
			return envoy.ParcelEventTypeDelivered
		case "38":
			return envoy.ParcelEventTypeAwaitingCustomerAction
		case "4X":
			return envoy.ParcelEventTypeTransferredToLocal
		}
		return envoy.ParcelEventTypeUnknown
	}
}
