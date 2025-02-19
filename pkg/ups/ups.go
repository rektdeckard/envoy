package ups

import (
	// "bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	// "sync"
	"time"

	"github.com/rektdeckard/envoy/pkg"
)

var (
	baseURL, _ = url.Parse("https://onlinetools.ups.com")
)

type UPSService struct {
	client    *http.Client
	apiKey    string
	apiSecret string
	token     *token
}

// Enforce that UPSService implements the Service interface
var _ envoy.Service = &UPSService{}

func NewUPSService(client *http.Client, apiKey, apiSecret string) *UPSService {
	return &UPSService{
		client:    client,
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func (s *UPSService) refreshToken() error {
	res := GetAccessToken(s.client, s.apiKey, s.apiSecret, nil, nil)

	if res.Error != "" {
		return fmt.Errorf("error getting access token: %s", res.Error)
	}

	expiresIn, err := strconv.Atoi(res.Response.Expires_in)
	if err != nil {
		return err
	}
	s.token = &token{
		value:      res.Response.Access_token,
		expiration: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	return nil
}

func (s *UPSService) Track(trackingNumbers []string) ([]envoy.Parcel, error) {
	const endpoint = "/api/track/v1/details/"

	if s.token == nil || !s.token.isValid() {
		if err := s.refreshToken(); err != nil {
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
		"Authorization":  []string{"Bearer " + s.token.value},
		"TransId":        []string{"1ZW701150378674373"},
		"TransactionSrc": []string{"envoy"},
	}

	var parcels []envoy.Parcel
	// wg := sync.WaitGroup{}

	for _, trackingNumber := range trackingNumbers {
		url := baseURL.ResolveReference(&url.URL{Path: endpoint + trackingNumber})
		url.RawQuery = params.Encode()

		req, err := http.NewRequest(http.MethodGet, url.String(), nil)
		if err != nil {
			return nil, err
		}

		req.Header = headers

		fmt.Printf("%+v\n\n", req)
		res, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		fmt.Println(string(body))

		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
		}

		var trackingRes trackingResponse
		if err := json.Unmarshal(body, &trackingRes); err != nil {
			return nil, err
		}
		// d, _ := json.MarshalIndent(trackingRes, "", "  ")
		// fmt.Println(string(d))

		for _, shipment := range trackingRes.TrackResponse.Shipment {
			for _, p := range shipment.Package {
				parcel := envoy.Parcel{
					Carrier:        envoy.CarrierUPS,
					TrackingNumber: p.TrackingNumber,
					TrackingURL:    fmt.Sprintf("https://www.ups.com/track?tracknum=%s", p.TrackingNumber),
				}

				var lastEvent *activity
				for _, a := range p.Activity {
					if lastEvent == nil || a.Date > lastEvent.Date {
						lastEvent = a
					}
					if a.Status.Type == "D" || a.Status.Code == "FS" {
						parcel.Delivered = true
					}
					parcel.TrackingEvents = append(parcel.TrackingEvents, envoy.ParcelEvent{
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

type token struct {
	value      string
	expiration time.Time
}

func (t *token) isValid() bool {
	return t.expiration.After(time.Now())
}

type trackingResponse struct {
	TrackResponse struct {
		Shipment []*shipment `json:"shipment"`
	} `json:"trackResponse"`
}

type shipment struct {
	InquiryNumber string      `json:"inquiryNumber"`
	Package       []*uPackage `json:"package"`
}

type uPackage struct {
	AccessPointInformation *accessPointInformation `json:"accessPointInformation"`
	Activity               []*activity             `json:"activity"`
	DeliveryDate           []*deliveryDate         `json:"deliveryDate"`
	DeliveryTime           *deliveryTime           `json:"deliveryTime"`
	TrackingNumber         string                  `json:"trackingNumber"`
}

type accessPointInformation struct {
	PickupByDate string `json:"pickupByDate"` // "YYYYMMDD"
}

type deliveryDate struct {
	Type string `json:"type"`
	Date string `json:"date"` // "YYYYMMDD"
}

type deliveryTime struct {
	Type    string `json:"type"`
	EndTime string `json:"endTime"` // "HHMMSS"
}

type activity struct {
	Location  *location `json:"location"`
	Status    *status   `json:"status"`
	Date      string    `json:"date"`      // "YYYYMMDD"
	Time      string    `json:"time"`      // "HHMMSS"
	GMTDate   string    `json:"gmtDate"`   // "YYYYMMDD"
	GMTOFFSET string    `json:"gmtOffset"` // "-HH:MM"
	GMTTime   string    `json:"gmtTime"`   // "HH:MM:SS"
}

func (a *activity) Timestamp() time.Time {
	t, _ := time.Parse("20060102150405", a.Date+a.Time)
	return t
}

type location struct {
	Address *address `json:"address"`
	SLIC    string   `json:"slic"`
}

type address struct {
	AddressLine1  string `json:"addressLine1"`
	AddressLine2  string `json:"addressLine2"`
	AddressLine3  string `json:"addressLine3"`
	City          string `json:"city"`
	StateProvince string `json:"stateProvince"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	PostalCode    string `json:"postalCode"`
}

func (a *address) String() string {
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

type status struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Code        string `json:"code"`
	StatusCode  string `json:"statusCode"`
}

func (s *status) ParcelEventType() envoy.ParcelEventType {
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
		return envoy.ParcelEventTypeUnknown
	}
}
