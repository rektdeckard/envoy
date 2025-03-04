package usps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rektdeckard/envoy/pkg"
)

var (
	BaseURL, _ = url.Parse("https://apis.usps.com")
)

type USPSService struct {
	Client         *http.Client
	ConsumerKey    string
	ConsumerSecret string
	Token          *Token
}

// Enforce that USPSService implements the Service interface
var _ envoy.Service = &USPSService{}

func NewUSPSService(client *http.Client, consumerKey, consumerSecret string) *USPSService {
	return &USPSService{
		Client:         client,
		ConsumerKey:    consumerKey,
		ConsumerSecret: consumerSecret,
	}
}

func (s *USPSService) Reauthenticate() error {
	const endpoint = "/oauth2/v3/token"

	data, err := json.Marshal(struct {
		GrantType    string `json:"grant_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scope        string `json:"scope"`
	}{
		GrantType:    "client_credentials",
		ClientID:     s.ConsumerKey,
		ClientSecret: s.ConsumerSecret,
		Scope:        "tracking",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	url := BaseURL.JoinPath(endpoint)
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Go-http-client/1.1 envoy")

	res, err := s.Client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return err
	}

	s.Token = &token
	return nil
}

func (s *USPSService) Track(trackingNumbers []string) ([]*envoy.Parcel, error) {
	responses, err := s.TrackRaw(trackingNumbers)
	if err != nil {
		return nil, err
	}

	parcels := make([]*envoy.Parcel, 0, len(responses))
	for _, res := range responses {
		p := &envoy.Parcel{
			Name:           res.TrackingNumber,
			Carrier:        envoy.CarrierUSPS,
			TrackingNumber: res.TrackingNumber,
			TrackingURL:    "https://tools.usps.com/go/TrackConfirmAction?tLabels=" + res.TrackingNumber,
			Data: &envoy.ParcelData{
				Delivered: strings.ToUpper(string(res.StatusCategory)) == "DELIVERED",
			},
		}
		for _, event := range res.TrackingEvents {
			p.Data.Events = append(p.Data.Events, envoy.ParcelEvent{
				Type:        event.ParcelEventType(),
				Description: string(event.EventType),
				Location:    event.LocationString(),
				Timestamp:   event.EventTimestamp.Time,
			})
		}
		parcels = append(parcels, p)
	}

	return parcels, nil
}

func (s *USPSService) TrackRaw(trackingNumbers []string) ([]*TrackingResponse, error) {
	const endpoint = "/tracking/v3/tracking"

	if s.Token == nil || !s.Token.IsValid() {
		if err := s.Reauthenticate(); err != nil {
			return nil, err
		}
	}

	params := url.Values{
		"expand": []string{"DETAIL"},
	}
	headers := http.Header{
		"Authorization": []string{"Bearer " + s.Token.Value},
	}

	wg := sync.WaitGroup{}
	mu := sync.Mutex{}
	var trackingResponses []*TrackingResponse

	for _, trackingNumber := range trackingNumbers {
		wg.Add(1)
		go func(tn string) {
			defer wg.Done()

			u := BaseURL.JoinPath(endpoint, tn)
			u.RawQuery = params.Encode()
			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				log.Printf("failed to create request: %v", err)
			}

			req.Header = headers

			res, err := s.Client.Do(req)
			if err != nil {
				log.Printf("failed to make request: %v", err)
			}

			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				log.Printf("failed to read response body: %v", err)
			}
			if res.StatusCode != http.StatusOK {
				log.Printf("unexpected status code: %d", res.StatusCode)
			}

			var trackingRes TrackingResponse
			if err := json.Unmarshal(body, &trackingRes); err != nil {
				log.Printf("failed to unmarshal response: %v", err)
				// TODO: return errors so TUI can display them
			} else {
				mu.Lock()
				trackingResponses = append(trackingResponses, &trackingRes)
				mu.Unlock()
			}

		}(trackingNumber)
	}

	wg.Wait()
	return trackingResponses, nil
}

// https://developers.usps.com/trackingv3#tag/Resources/operation/get-package-tracking
type TrackingResponse struct {
	TrackingNumber              string           `json:"trackingNumber"`
	AdditionalInfo              string           `json:"additionalInfo"`
	ADPScripting                string           `json:"ADPScripting"`
	ArchiveRestoreInfo          string           `json:"archiveRestoreInfo"`
	AssociatedLabel             string           `json:"associatedLabel"`
	CarrierRelease              bool             `json:"carrierRelease"`
	MailClass                   MailClass        `json:"mailClass"`
	DestinationCity             string           `json:"destinationCity"`
	DestinationCountryCode      string           `json:"destinationCountryCode"`
	DestinationState            string           `json:"destinationState"`
	DestinationZIP              string           `json:"destinationZIP"`
	EditedLabelID               string           `json:"editedLabelId"`
	EmailEnabled                envoy.BoolString `json:"emailEnabled"`
	EndOfDay                    string           `json:"endOfDay"`
	ESOFEligible                bool             `json:"eSOFEligible"`
	ExpectedDeliveryTimestamp   time.Time        `json:"expectedDeliveryTimestamp"`
	ExpectedDeliveryType        string           `json:"expectedDeliveryType"`
	GuaranteedDeliveryTimestamp time.Time        `json:"guaranteedDeliveryTimestamp"`
	GuaranteedDetails           string           `json:"guaranteedDetails"`
	ItemShape                   ItemShape        `json:"itemShape"`
	KahalaIndicator             envoy.BoolString `json:"kahalaIndicator"`
	MailType                    MailType         `json:"mailType"`
	// Deprecated: use [TrackingResponse.MailPieceIntakeDate] instead
	ApproximateIntakeDate string `json:"approximateIntakeDate"`
	MailPieceIntakeDate   string `json:"mailPieceIntakeDate"`
	// Deprecated: use [TrackingResponse.UniqureMailPeiceID] instead
	UniqueTrackingID       string           `json:"uniqueTrackingId"`
	UniqueMailPieceID      string           `json:"uniqueMailPieceId"`
	OnTime                 bool             `json:"onTime"`
	OriginCity             string           `json:"originCity"`
	OriginCountry          string           `json:"originCountry"`
	OriginState            string           `json:"originState"`
	OriginZIP              string           `json:"originZIP"`
	ProofOfDeliveryEnabled envoy.BoolString `json:"proofOfDeliveryEnabled"`
	// Deprecated: use [TrackingResponse.PredictedDeliveryWindowStartTime] and [TrackingResponse.PredictedDeliveryWindowEndTime] instead
	PredictedDeliveryTimestamp               time.Time                   `json:"predictedDeliveryTimestamp"`
	PredictedDeliveryDate                    string                      `json:"predictedDeliveryDate"`
	PredictedDeliveryWindowStartTime         string                      `json:"predictedDeliveryWindowStartTime"`
	PredictedDeliveryWindowEndTime           string                      `json:"predictedDeliveryWindowEndTime"`
	RelatedReturnReceiptID                   string                      `json:"relatedReturnReceiptID"`
	RedeliveryEnabled                        bool                        `json:"redeliveryEnabled"`
	EnabledNotificationRequests              *NotificationRequests       `json:"enabledNotificationRequests"`
	RestoreEnabled                           envoy.BoolString            `json:"restoreEnabled"`
	ReturnDateNotice                         string                      `json:"returnDateNotice"`
	RRAMEnabled                              envoy.BoolString            `json:"RRAMEnabled"`
	Services                                 []ExtraService              `json:"services"`
	ServiceTypeCode                          ServiceTypeCode             `json:"serviceTypeCode"`
	Status                                   Status                      `json:"status"`
	StatusCategory                           StatusCategory              `json:"statusCategory"`
	StatusSummary                            string                      `json:"statusSummary"`
	TableCode                                TableCode                   `json:"tableCode"`
	TrackingProofOfDeliveryEnabled           bool                        `json:"trackingProofOfDeliveryEnabled"`
	ValueOfArticle                           string                      `json:"valueOfArticle"`
	ExtendedRetentionPurchasedCode           string                      `json:"extendedRetentionPurchasedCode"`
	ExtendedRetentionExtraServiceCodeOptions []*ExtendedRetentionOptions `json:"extendedRetentionExtraServiceCodeOptions"`
	TrackingEvents                           []*TrackingEvent            `json:"trackingEvents"`
}

type MailClass string

const (
	MailClassBoundPrintedMatter               MailClass = "BOUND_PRINTED_MATTER"
	MailClassCriticalMail                     MailClass = "CRITICAL_MAIL"
	MailClassDomesticMatterForTheBlind        MailClass = "DOMESTIC_MATTER_FOR_THE_BLIND"
	MailClassFirstClassMail                   MailClass = "FIRST-CLASS_MAIL"
	MailClassFirstClassPackageInternational   MailClass = "FIRST-CLASS_PACKAGE_INTERNATIONAL_SERVICE"
	MailClassGlobalExpressGuaranteed          MailClass = "GLOBAL_EXPRESS_GUARANTEED"
	MailClassLibraryMail                      MailClass = "LIBRARY_MAIL"
	MailClassMediaMail                        MailClass = "MEDIA_MAIL"
	MailClassParcelSelect                     MailClass = "PARCEL_SELECT"
	MailClassParcelSelectLightweight          MailClass = "PARCEL_SELECT_LIGHTWEIGHT"
	MailClassPriorityMail                     MailClass = "PRIORITY_MAIL"
	MailClassPriorityMailExpress              MailClass = "PRIORITY_MAIL_EXPRESS"
	MailClassPriorityMailExpressInternational MailClass = "PRIORITY_MAIL_EXPRESS_INTERNATIONAL"
	MailClassPriorityMailGuaranteed           MailClass = "PRIORITY_MAIL_GUARANTEED"
	MailClassPriorityMailInternational        MailClass = "PRIORITY_MAIL_INTERNATIONAL_PARCELS"
	MailClassPriorityMailSameDay              MailClass = "PRIORITY_MAIL_SAME_DAY"
	MailClassUSPSMarketingMail                MailClass = "USPS_MARKETING_MAIL"
	MailClassUSPSRetailGround                 MailClass = "USPS_RETAIL_GROUND"
)

type ItemShape string

const (
	ItemShapeLetter  ItemShape = "LETTER"
	ItemShapeFlat    ItemShape = "FLAT"
	ItemShapeParcel  ItemShape = "PARCEL"
	ItemShapeUnknown ItemShape = "UNKNOWN"
)

type MailType string

const (
	MailTypeInternationalInbound  MailType = "INTERNATIONAL_INBOUND"
	MailTypeInternationalOutbound MailType = "INTERNATIONAL_OUTBOUND"
	MailTypeDomestic              MailType = "DOMESTIC_MAIL"
	MailTypeUnknown               MailType = "UNKNOWN"
)

type NotificationRequests struct {
	SMS   *BasicNotificationOptions `json:"SMS"`
	EMail struct {
		BasicNotificationOptions
		FirstDisplayable bool `json:"firstDisplayable"`
		OtherActivity    bool `json:"otherActivity"`
	} `json:"EMail"`
}

type BasicNotificationOptions struct {
	FutureDelivery bool `json:"futureDelivery"`
	AlertDelivery  bool `json:"alertDelivery"`
	TodayDelivery  bool `json:"todayDelivery"`
	UP             bool `json:"UP"`
	DND            bool `json:"DND"`
}

// See USPS Pub 199 Appendix I
// https://postalpro.usps.com/pub199
type ServiceTypeCode string

const (
// TODO: translate from scratchfile
)

// See USPS Pub 199 Appendix J Table 7
// https://postalpro.usps.com/pub199
type ExtraService string

const (
	// TODO: add all additional services
	ExtraServiceUSPSLabelDelivery                                      ExtraService = "415"
	ExtraServiceParcelReturnService                                    ExtraService = "420"
	ExtraServiceOpenAndDistribute                                      ExtraService = "430"
	ExtraServiceUSPSReturns                                            ExtraService = "452"
	ExtraServiceHAZMATAirEligibleEthanolPackage                        ExtraService = "810"
	ExtraServiceHAZMATClass1ToyPropellantSafetyFusePackage             ExtraService = "811"
	ExtraServiceHAZMATClass3FlammableLiquidPackage                     ExtraService = "812"
	ExtraServiceHAZMATClass7RadioactiveMaterialsPackage                ExtraService = "813"
	ExtraServiceHAZMATClass8CorrosiveMaterialsPackage                  ExtraService = "814"
	ExtraServiceHAZMATClass8NonspillableWetBatteryPackage              ExtraService = "815"
	ExtraServiceHAZMATClass9LithiumBatteryMarkedGroundOnlyPackage      ExtraService = "816"
	ExtraServiceHAZMATClass9LithiumBatteryReturnsPackage               ExtraService = "817"
	ExtraServiceHAZMATClass9LithiumBatteriesMarkedPackage              ExtraService = "818"
	ExtraServiceHAZMATClass9DryIcePackage                              ExtraService = "819"
	ExtraServiceHAZMATClass9LithiumBatteriesUnmarkedPackage            ExtraService = "820"
	ExtraServiceHAZMATClass9MagnetizedMaterialsPackage                 ExtraService = "821"
	ExtraServiceHAZMATDivision4_1FlammableSolidsOrSafetyMatchesPackage ExtraService = "822"
	ExtraServiceHAZMATDivision5_1OxidizersPackage                      ExtraService = "823"
	ExtraServiceHAZMATDivision5_2OrganicPeroxidesPackage               ExtraService = "824"
	ExtraServiceHAZMATDivision6_1ToxicMaterialsPackage                 ExtraService = "825"
	ExtraServiceHAZMATDivision6_2InfectiousSubstancesPackage           ExtraService = "826"
	ExtraServiceHAZMATExceptedQuantityProvisionPackage                 ExtraService = "827"
	ExtraServiceHAZMATGroundOnly                                       ExtraService = "828"
	ExtraServiceHAZMATID8000ConsumerCommodityPackage                   ExtraService = "829"
	ExtraServiceHAZMATLightersPackage                                  ExtraService = "830"
	ExtraServiceHAZMATLTDQTYGroundPackage                              ExtraService = "831"
	ExtraServiceHAZMATSmallQuantityProvisionPackage                    ExtraService = "832"
	ExtraServicePerishableMaterial                                     ExtraService = "853"
	ExtraServiceLiveAnimalsTransportationFee                           ExtraService = "856"
	ExtraServiceHazardousMaterial                                      ExtraService = "857"
	ExtraServiceCrematedRemains                                        ExtraService = "858"
	ExtraServiceCrematedRemainsInternational                           ExtraService = "859"
	ExtraServiceNonStandardDimLenGT22In                                ExtraService = "881"
	ExtraServiceNonStandardDimLenGT30In                                ExtraService = "882"
	ExtraServiceNonStandardCubicDimGT2CuFt                             ExtraService = "883"
	ExtraServiceNonStandardDimLenGT22InCubicDimGT2CuFt                 ExtraService = "884"
	ExtraServiceNonStandardDimLenGT30InCubicDimGT2CuFt                 ExtraService = "885"
	ExtraServiceCertifiedMail                                          ExtraService = "910"
	ExtraServiceCertifiedMailRestrictedDelivery                        ExtraService = "911"
	ExtraServiceCertifiedMailAdultSignatureRequired                    ExtraService = "912"
	ExtraServiceCertifiedMailAdultSignatureRestrictedDelivery          ExtraService = "913"
	ExtraServiceCOD                                                    ExtraService = "915"
	ExtraServiceCODRestrictedDelivery                                  ExtraService = "917"
	ExtraServiceUSPSTracking                                           ExtraService = "920"
	ExtraServiceSignatureConfirmation                                  ExtraService = "921"
	ExtraServiceAdultSignatureRequired21OrOver                         ExtraService = "922"
	ExtraServiceAdultSignatureRestrictedDelivery21OrOver               ExtraService = "923"
	ExtraServiceSignatureConfirmationRestrictedDelivery                ExtraService = "924"
	ExtraServicePriorityMailExpressMerchandiseInsurance                ExtraService = "925"
	ExtraServiceInsuranceLE500                                         ExtraService = "930"
	ExtraServiceInsuranceGT500                                         ExtraService = "931"
	ExtraServiceInsuranceRestrictedDelivery                            ExtraService = "934"
	ExtraServiceRegisteredMail                                         ExtraService = "940"
	ExtraServiceRegisteredMailRestrictedDelivery                       ExtraService = "941"
	ExtraServiceReturnReceipt                                          ExtraService = "955"
	ExtraServiceReturnReceiptElectronic                                ExtraService = "957"
	ExtraServiceLiveAnimalAndPerishableHandlingFee                     ExtraService = "972"
	ExtraServiceSignatureRequested                                     ExtraService = "981"
	ExtraServiceHoldForPickup                                          ExtraService = "985"
	ExtraServicePOToAddressee                                          ExtraService = "986"
)

type Status string

const (
// TODO: how is this different from an event code?
)

type StatusCategory string

type TableCode string

type ExtendedRetentionOptions struct {
	// TODO: where is this defined?
}

type TrackingEvent struct {
	EventType       TrackingEventType   `json:"eventType"`
	EventTimestamp  envoy.LocalDateTime `json:"eventTimestamp"`
	GMTTimestamp    time.Time           `json:"GMTTimestamp"`
	GMTOffset       string              `json:"GMTOffset"`
	EventCountry    string              `json:"eventCountry"`
	EventCity       string              `json:"eventCity"`
	EventState      string              `json:"eventState"`
	EventZIP        string              `json:"eventZIP"`
	Firm            string              `json:"firm"`
	Name            string              `json:"name"`
	AuthorizedAgent envoy.BoolString    `json:"authorizedAgent"`
	EventCode       TrackingEventCode   `json:"eventCode"`
	ActionCode      ActionCode          `json:"actionCode"`
	ReasonCode      ReasonCode          `json:"reasonCode"`
}

type TrackingEventCode string

type TrackingEventType string

func (e *TrackingEvent) ParcelEventType() envoy.ParcelEventType {
	switch e.EventCode {
	case "DELIVERY":
		return envoy.ParcelEventTypeDelivered
	case "ARRIVAL":
		return envoy.ParcelEventTypeArrived
	case "DEPARTURE":
		return envoy.ParcelEventTypeDeparted
	case "OUT_FOR_DELIVERY":
		return envoy.ParcelEventTypeOutForDelivery
	default:
		return envoy.ParcelEventTypeUnknown
	}
}

func (e *TrackingEvent) LocationString() string {
	sb := strings.Builder{}
	if e.EventCity != "" {
		sb.WriteString(e.EventCity)
		if e.EventState != "" {
			sb.WriteString(", ")
		}
	}
	sb.WriteString(e.EventState)
	if e.EventZIP != "" {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(e.EventZIP)
	}
	if e.EventCountry != "" && e.EventCountry != "US" {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(e.EventCountry)
	}
	if sb.Len() == 0 {
		return "—"
	}
	return strings.ToUpper(sb.String())
}

type ActionCode string

type ReasonCode string

const (
// TODO: add all event types
)

type Token struct {
	Value      string
	PublicKey  string
	Expiration time.Time
}

func (t *Token) IsValid() bool {
	return t.Expiration.After(time.Now())
}

func (t *Token) UnmarshalJSON(data []byte) error {
	var raw struct {
		AccessToken     string `json:"access_token"`
		TokenType       string `json:"token_type"`
		IssuedAt        int64  `json:"issued_at"`
		ExpiresIn       int    `json:"expires_in"`
		Status          string `json:"status"`
		Scope           string `json:"scope"`
		Issuer          string `json:"issuer"`
		ClientID        string `json:"client_id"`
		ApplicationName string `json:"application_name"`
		APIProducts     string `json:"api_products"`
		PublicKey       string `json:"public_key"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.Status != "approved" {
		return fmt.Errorf("token status is not approved: %s", raw.Status)
	}

	if !strings.Contains(raw.Scope, "tracking") {
		return fmt.Errorf("token scope does not include tracking: %s", raw.Scope)
	}

	expiration := time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)

	t.Value = raw.AccessToken
	t.PublicKey = raw.PublicKey
	t.Expiration = expiration

	return nil
}
