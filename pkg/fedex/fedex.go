package fedex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rektdeckard/envoy/pkg"
)

var (
	BaseURL, _ = url.Parse("https://apis.fedex.com")
)

type FedexService struct {
	Client    *http.Client
	APIKey    string
	APISecret string
	Token     *Token
}

// Enforce that FedexService implements the Service interface
var _ envoy.Service = &FedexService{}

func NewFedexService(client *http.Client, apiKey, apiSecret string) *FedexService {
	return &FedexService{
		Client:    client,
		APIKey:    apiKey,
		APISecret: apiSecret,
	}
}

func (s *FedexService) Reauthenticate() error {
	const endpoint = "/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", s.APIKey)
	data.Set("client_secret", s.APISecret)

	url := BaseURL.JoinPath(endpoint)
	req, err := http.NewRequest("POST", url.String(), strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

func (s *FedexService) TrackRaw(trackingNumbers []string) (*TrackingResponse, error) {
	const endpoint = "/track/v1/trackingnumbers"

	if s.Token == nil || !s.Token.IsValid() {
		if err := s.Reauthenticate(); err != nil {
			return nil, err
		}
	}

	data := newTrackingRequest(trackingNumbers)
	reqBody, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	url := BaseURL.JoinPath(endpoint)
	req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Token.Value)
	req.Header.Set("x-locale", "en_US")

	res, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	var trackingRes TrackingResponse
	if err := json.Unmarshal(body, &trackingRes); err != nil {
		return nil, err
	}
	return &trackingRes, nil
}

func (s *FedexService) Track(trackingNumbers []string) ([]*envoy.Parcel, error) {
	trackingRes, err := s.TrackRaw(trackingNumbers)
	if err != nil {
		return nil, err
	}

	var parcels []*envoy.Parcel
	for _, r := range trackingRes.Output.CompleteTrackResults {
		parcel := envoy.Parcel{
			Name:           r.TrackingNumer, // TODO: derive name
			Carrier:        envoy.CarrierFedEx,
			TrackingNumber: r.TrackingNumer,
			TrackingURL: fmt.Sprintf(
				"https://www.fedex.com/apps/fedextrack/?tracknumbers=%s",
				r.TrackingNumer,
			),
			Data: &envoy.ParcelData{},
		}

		for _, r := range r.TrackResults {
			if r.ScanEvents == nil || len(r.ScanEvents) == 0 {
				continue
			}
			var lastEvent *ScanEvent
			for _, e := range r.ScanEvents {
				if lastEvent == nil || e.Date.Time.After(lastEvent.Date.Time) {
					lastEvent = e
				}
				if e.EventType == "DL" {
					parcel.Data.Delivered = true
				}
				parcel.Data.Events = append(parcel.Data.Events, envoy.ParcelEvent{
					Timestamp:   e.Date.Time,
					Description: e.EventDescription,
					Location:    e.ScanLocation.String(),
					Type:        e.EventType.ParcelEventType(),
				})
			}
		}

		parcels = append(parcels, &parcel)
	}

	return parcels, nil
}

type request struct {
	TrackingInfo         []*trackingInfo `json:"trackingInfo"`
	IncludeDetailedScans bool            `json:"includeDetailedScans"`
}

type trackingInfo struct {
	ShipDateBegin      string              `json:"shipDateBegin,omitempty"`
	ShipDateEnd        string              `json:"shipDateEnd,omitempty"`
	TrackingNumberInfo *TrackingNumberInfo `json:"trackingNumberInfo"`
}

type TrackingNumberInfo struct {
	TrackingNumber         string `json:"trackingNumber"`
	TrackingNumberUniqueId string `json:"trackingNumberUniqueId,omitempty"`
	CarrierCode            string `json:"carrierCode,omitempty"`
}

func newTrackingRequest(trackingNumbers []string) *request {
	tr := &request{
		IncludeDetailedScans: true,
	}

	for _, tn := range trackingNumbers {
		tr.TrackingInfo = append(tr.TrackingInfo, &trackingInfo{
			// ShipDateBegin: "2021-01-01",
			// ShipDateEnd:   "2021-12-31",
			TrackingNumberInfo: &TrackingNumberInfo{
				TrackingNumber: tn,
				// CarrierCode:    "FDXE",
			},
		})
	}

	return tr
}

// https://developer.fedex.com/api/en-us/catalog/track/v1/docs.html#operation/Track%20by%20Tracking%20Number
type TrackingResponse struct {
	TransactionId         string          `json:"transactionId"`
	CustomerTransactionId string          `json:"customerTransactionId"`
	Output                *TrackingOutput `json:"output"`
}

type TrackingOutput struct {
	CompleteTrackResults []*CompleteTrackResult `json:"completeTrackResults"`
	Alerts               []*Alert               `json:"alerts"`
}

type CompleteTrackResult struct {
	TrackingNumer string          `json:"trackingNumber"`
	TrackResults  []*TrackResults `json:"trackResults"`
}

type TrackResults struct {
	TrackingNumberInfo            *TrackingNumberInfo     `json:"trackingNumberInfo"`
	AdditionalTrackingInfo        *AdditionalTrackingInfo `json:"additionalTrackingInfo"`
	InformationNotes              []*InformationNote      `json:"informationNotes"`
	ScanEvents                    []*ScanEvent            `json:"scanEvents"`
	DateAndTimes                  []*DateAndTime          `json:"dateAndTimes"`
	AvailableImages               []*AvailableImage       `json:"availableImages"`
	MeterNumber                   string                  `json:"meterNumber"`
	OriginLocation                *Location               `json:"originLocation"`
	DestinationLocation           *DestinationLocation    `json:"destinationLocation"`
	HoldAtLocation                *Location               `json:"holdAtLocation"`
	GoodsClassificationCode       string                  `json:"goodsClassificationCode"`
	DeliveryDetails               *DeliveryDetails        `json:"deliveryDetails"`
	DistanceToDestination         envoy.Dimensioned       `json:"distanceToDestination"`
	CustomDeliveryOptions         []*CustomDeliveryOption `json:"customDeliveryOptions"`
	SpecialHandlings              []*SpecialHandling      `json:"specialHandlings"`
	EstimatedDeliveryTimeWindow   *DeliveryWindow         `json:"estimatedDeliveryTimeWindow"`
	PackageDetails                *PackageDetails         `json:"packageDetails"`
	ShipmentDetails               *ShipmentDetails        `json:"shipmentDetails"`
	PieceCounts                   []*PieceCount           `json:"pieceCounts"`
	ReturnDetail                  *ReturnDetail           `json:"returnDetail"`
	ServiceDetail                 *ServiceDetail          `json:"serviceDetail"`
	ConsolidationDetail           []*ConsolidationDetail  `json:"consolidationDetail"`
	LastStatusDetail              *StatusDetail           `json:"lastStatusDetail"`
	LastUpdatedDestinationAddress *Address                `json:"lastUpdatedDestinationAddress"`
	ShipperInformation            struct {
		Contact *Contact `json:"contact"`
		Address *Address `json:"address"`
	} `json:"shipperInformation"`
	RecipientInformation struct {
		Contact *Contact `json:"contact"`
		Address Address  `json:"address"`
	} `json:"recipientInformation"`
	StandardTransitTimeWindow *DeliveryWindow      `json:"standardTransitTimeWindow"`
	ReasonDetail              *ReasonDetail        `json:"reasonDetail"`
	ServiceCommitMessage      ServiceCommitMessage `json:"serviceCommitMessage"`
	AvailableNotifications    []string             `json:"availableNotifications"`
	Error                     *ErrorInfo           `json:"error"`
}

type ShipmentDetails struct {
	Contents               []*ShipmentContent  `json:"contents"`
	BeforePossessionStatus bool                `json:"beforePossessionStatus"`
	Weight                 []envoy.Dimensioned `json:"weight"`
	ContentPieceCount      string              `json:"contentPieceCount"`
	SplitShipments         []*SplitShipment    `json:"splitShipments"`
}

type SplitShipment struct {
	PieceCount        string    `json:"pieceCount"`
	StatusDescription string    `json:"statusDescription"`
	Timestamp         time.Time `json:"timestamp"`
	StatusCode        string    `json:"statusCode"`
}

type ShipmentContent struct {
	ItemNumber       string `json:"itemNumber"`
	ReceivedQuantity string `json:"receivedQuantity"`
	Description      string `json:"description"`
	PartNumber       string `json:"partNumber"`
}

type AdditionalTrackingInfo struct {
	HasAssociatedShipments bool                 `json:"hasAssociatedShipments"`
	Nickname               string               `json:"nickname"`
	PackageIdentifiers     []*PackageIdentifier `json:"packageIdentifiers"`
	ShipmentNotes          string               `json:"shipmentNotes"`
}

type PackageIdentifier struct {
	Type                   PackageIdentifierType `json:"type"`
	Values                 []string              `json:"values"`
	TrackingNumberUniqueId string                `json:"trackingNumberUniqueId"`
}

type PackageIdentifierType string

const (
	PackageIdentifierTypeBillOfLading                    PackageIdentifierType = "BILL_OF_LADING"
	PackageIdentifierTypeCodReturnTrackingNumber         PackageIdentifierType = "COD_RETURN_TRACKING_NUMBER"
	PackageIdentifierTypeCustomerAuthorizationNumber     PackageIdentifierType = "CUSTOMER_AUTHORIZATION_NUMBER"
	PackageIdentifierTypeCustomerReference               PackageIdentifierType = "CUSTOMER_REFERENCE"
	PackageIdentifierTypeDepartment                      PackageIdentifierType = "DEPARTMENT"
	PackageIdentifierTypeDocumentAirwayBill              PackageIdentifierType = "DOCUMENT_AIRWAY_BILL"
	PackageIdentifierTypeExpressAlternateReference       PackageIdentifierType = "EXPRESS_ALTERNATE_REFERENCE"
	PackageIdentifierTypeFedexOfficeJobOrderNumber       PackageIdentifierType = "FEDEX_OFFICE_JOB_ORDER_NUMBER"
	PackageIdentifierTypeFreeFormReference               PackageIdentifierType = "FREE_FORM_REFERENCE"
	PackageIdentifierTypeGroundInternational             PackageIdentifierType = "GROUND_INTERNATIONAL"
	PackageIdentifierTypeGroundShipmentID                PackageIdentifierType = "GROUND_SHIPMENT_ID"
	PackageIdentifierTypeGroupMPS                        PackageIdentifierType = "GROUP_MPS"
	PackageIdentifierTypeInternationalDistribution       PackageIdentifierType = "INTERNATIONAL_DISTRIBUTION"
	PackageIdentifierTypeInvoice                         PackageIdentifierType = "INVOICE"
	PackageIdentifierTypeJobGlobalTrackingNumber         PackageIdentifierType = "JOB_GLOBAL_TRACKING_NUMBER"
	PackageIdentifierTypeOrderGlobalTrackingNumber       PackageIdentifierType = "ORDER_GLOBAL_TRACKING_NUMBER"
	PackageIdentifierTypeOrderToPayNumber                PackageIdentifierType = "ORDER_TO_PAY_NUMBER"
	PackageIdentifierTypeOutboundLinkToReturn            PackageIdentifierType = "OUTBOUND_LINK_TO_RETURN"
	PackageIdentifierTypePartNumber                      PackageIdentifierType = "PART_NUMBER"
	PackageIdentifierTypePartnerCarrierNumber            PackageIdentifierType = "PARTNER_CARRIER_NUMBER"
	PackageIdentifierTypePurchaseOrder                   PackageIdentifierType = "PURCHASE_ORDER"
	PackageIdentifierTypeRerouteTrackingNumber           PackageIdentifierType = "REROUTE_TRACKING_NUMBER"
	PackageIdentifierTypeReturnMaterialsAuthorization    PackageIdentifierType = "RETURN_MATERIALS_AUTHORIZATION"
	PackageIdentifierTypeReturnedToShipperTrackingNumber PackageIdentifierType = "RETURNED_TO_SHIPPER_TRACKING_NUMBER"
	PackageIdentifierTypeShipperReference                PackageIdentifierType = "SHIPPER_REFERENCE"
	PackageIdentifierTypeStandardMPS                     PackageIdentifierType = "STANDARD_MPS"
	PackageIdentifierTypeTrackingControlNumber           PackageIdentifierType = "TRACKING_CONTROL_NUMBER"
	PackageIdentifierTypeTrackingNumberOrDoorTag         PackageIdentifierType = "TRACKING_NUMBER_OR_DOORTAG"
	PackageIdentifierTypeTransborderDistribution         PackageIdentifierType = "TRANSBORDER_DISTRIBUTION"
	PackageIdentifierTypeTransportationControlNumber     PackageIdentifierType = "TRANSPORTATION_CONTROL_NUMBER"
	PackageIdentifierTypeVirtualConsolidation            PackageIdentifierType = "VIRTUAL_CONSOLIDATION"
)

type ConsolidationDetail struct {
	TimeStamp       time.Time              `json:"timeStamp"`
	ConsolidationId string                 `json:"consolidationId"`
	ReasonDetail    ReasonDetail           `json:"reasonDetail"`
	PackageCount    int                    `json:"packageCount"`
	EventType       ConsolidationEventType `json:"eventType"`
}

type ConsolidationEventType string

const (
	ConsolidationEventTypeAdded    ConsolidationEventType = "ADDED_TO_CONSOLIDATION"
	ConsolidationEventTypeRemoved  ConsolidationEventType = "REMOVED_FROM_CONSOLIDATION"
	ConsolidationEventTypeExcluded ConsolidationEventType = "EXCLUDED_FROM_CONSOLIDATION"
)

type ReasonDetail struct {
	Description string `json:"description"`
	Type        string `json:"type"`
}

type ReturnDetail struct {
	AuthorizationName string       `json:"authorizationName"`
	ReasonDetail      ReasonDetail `json:"reasonDetail"`
}

type ServiceDetail struct {
	Description      string      `json:"description"`
	ShortDescription string      `json:"shortDescription"`
	Type             ServiceType `json:"type"`
}

type ServiceType string

// https://developer.fedex.com/api/en-us/guides/api-reference.html#servicetypes
const (
	ServiceTypeFedexInternationalPriorityExpress      ServiceType = "FEDEX_INTERNATIONAL_PRIORITY_EXPRESS"
	ServiceTypeFedexInternationalFirst                ServiceType = "FEDEX_INTERNATIONAL_FIRST"
	ServiceTypeFedexInternationalPriority             ServiceType = "FEDEX_INTERNATIONAL_PRIORITY"
	ServiceTypeFedexInternationalEconomy              ServiceType = "INTERNATIONAL_ECONOMY"
	ServiceTypeFedexGround                            ServiceType = "FEDEX_GROUND"
	ServiceTypeFedexFirstOvernight                    ServiceType = "FIRST_OVERNIGHT"
	ServiceTypeFedexFirstOvernightFreight             ServiceType = "FEDEX_FIRST_FREIGHT"
	ServiceTypeFedex1DayFreight                       ServiceType = "FEDEX_1_DAY_FREIGHT"
	ServiceTypeFedex2DayFreight                       ServiceType = "FEDEX_2_DAY_FREIGHT"
	ServiceTypeFedex3DayFreight                       ServiceType = "FEDEX_3_DAY_FREIGHT"
	ServiceTypeFedexInternationalPriorityFreight      ServiceType = "INTERNATIONAL_PRIORITY_FREIGHT"
	ServiceTypeFedexInternationalEconomyFreight       ServiceType = "INTERNATIONAL_ECONOMY_FREIGHT"
	ServiceTypeFedexInternationalDeferredFreight      ServiceType = "FEDEX_INTERNATIONAL_DEFERRED_FREIGHT"
	ServiceTypeFedexInternationalPriorityDistribution ServiceType = "INTERNATIONAL_PRIORITY_DISTRIBUTION"
	ServiceTypeFedexInternationalDistributionFreight  ServiceType = "INTERNATIONAL_DISTRIBUTION_FREIGHT"
	ServiceTypeInternationalGroundDistribution        ServiceType = "INTL_GROUND_DISTRIBUTION"
	ServiceTypeFedexHomeDelivery                      ServiceType = "GROUND_HOME_DELIVERY"
	ServiceTypeFedexGroundEconomy                     ServiceType = "SMART_POST"
	ServiceTypeFedexPriorityOvernight                 ServiceType = "PRIORITY_OVERNIGHT"
	ServiceTypeFedexStandardOvernight                 ServiceType = "STANDARD_OVERNIGHT"
	ServiceTypeFedex2Day                              ServiceType = "FEDEX_2_DAY"
	ServiceTypeFedex2DayAM                            ServiceType = "FEDEX_2_DAY_AM"
	ServiceTypeFedexExpressSaver                      ServiceType = "FEDEX_EXPRESS_SAVER"
	ServiceTypeFedexSameDay                           ServiceType = "SAME_DAY"
	ServiceTypeFedexSameDayCity                       ServiceType = "SAME_DAY_CITY"
)

type DestinationLocation struct {
	LocationId                string                     `json:"locationId"`
	LocationContactAndAddress *LocationContactAndAddress `json:"locationContactAndAddress"`
	LocationType              FedexLocationType          `json:"locationType"`
}

type LocationContactAndAddress struct {
	Address *Address `json:"address"`
}

type FedexLocationType string

const (
	FedexLocationTypeAuthorizedShipCenter FedexLocationType = "FEDEX_AUTHORIZED_SHIP_CENTER"
	FedexLocationTypeOffice               FedexLocationType = "FEDEX_OFFICE"
	FedexLocationTypeSelfServiceLocation  FedexLocationType = "FEDEX_SELF_SERVICE_LOCATION"
	FedexLocationTypeGroundTerminal       FedexLocationType = "FEDEX_GROUND_TERMINAL"
	FedexLocationTypeOnsite               FedexLocationType = "FEDEX_ONSITE"
	FedexLocationTypeExpressStation       FedexLocationType = "FEDEX_EXPRESS_STATION"
	FedexLocationTypeFacility             FedexLocationType = "FEDEX_FACILITY"
	FedexLocationTypeFreightServiceCenter FedexLocationType = "FEDEX_FREIGHT_SERVICE_CENTER"
	FedexLocationTypeHomeDeliveryStation  FedexLocationType = "FEDEX_HOME_DELIVERY_STATION"
	FedexLocationTypeShipAndGet           FedexLocationType = "FEDEX_SHIP_AND_GET"
	FedexLocationTypeShipsite             FedexLocationType = "FEDEX_SHIPSITE"
	FedexLocationTypeSmartPostHub         FedexLocationType = "FEDEX_SMART_POST_HUB"
)

type StatusDetail struct {
	Code             string             `json:"code"`
	DerivedCode      string             `json:"derivedCode"`
	Description      string             `json:"description"`
	ScanLocation     *Address           `json:"scanLocation"`
	DelayDetail      *DelayDetail       `json:"delayDetail"`
	AncillaryDetails []*AncillaryDetail `json:"ancillaryDetails"`
	StatusByLocale   string             `json:"statusByLocale"`
}

type EventType string

func (d *EventType) ParcelEventType() envoy.ParcelEventType {
	if d == nil {
		return envoy.ParcelEventTypeUnknown
	}
	switch string(*d) {
	case "OC":
		return envoy.ParcelEventTypeOrderConfirmed
	case "PU":
		return envoy.ParcelEventTypePickedUp
	case "AO":
		return envoy.ParcelEventTypeAssertOnTime
	case "DP":
		return envoy.ParcelEventTypeDeparted
	case "AR":
		return envoy.ParcelEventTypeArrived
	case "OD":
		return envoy.ParcelEventTypeOutForDelivery
	case "DL":
		return envoy.ParcelEventTypeDelivered
	default:
		return envoy.ParcelEventTypeUnknown
	}
}

type Contact struct{}

type Address struct {
	AddressClassification string   `json:"addressClassification"`
	Residential           bool     `json:"residential"`
	StreetLines           []string `json:"streetLines"`
	City                  string   `json:"city"`
	StateOrProvinceCode   string   `json:"stateOrProvinceCode"`
	PostalCode            string   `json:"postalCode"`
	CountryCode           string   `json:"countryCode"`
	CountryName           string   `json:"countryName"`
}

func (a *Address) String() string {
	sb := strings.Builder{}
	if a.City != "" {
		sb.WriteString(a.City)
		if a.StateOrProvinceCode != "" {
			sb.WriteString(", ")
		}
	}
	sb.WriteString(a.StateOrProvinceCode)
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

type AncillaryDetail struct {
	Reason            string `json:"reason"`
	ReasonDesctiption string `json:"reasonDescription"`
	Action            string `json:"action"`
	ActionDescription string `json:"actionDescription"`
}

type DelayDetail struct {
	Type    DelayType    `json:"type"`
	SubType DelaySubType `json:"subType"`
	Status  DelayStatus  `json:"status"`
}

type DelayType string

const (
	DelayTypeWeather     DelayType = "WEATHER"
	DelayTypeOperational DelayType = "OPERATIONAL"
	DelayTypeLocal       DelayType = "LOCAL"
	DelayTypeGeneral     DelayType = "GENERAL"
	DelayTypeClearance   DelayType = "CLEARANCE"
)

type DelaySubType string

const (
	DelaySubTypeSnow          DelaySubType = "SNOW"
	DelaySubTypeTornado       DelaySubType = "TORNADO"
	DelaySubTypeEarthquakeEtc DelaySubType = "EARTHQUAKE etc"
)

type DelayStatus string

const (
	DelayStatusDelayed DelayStatus = "DELAYED"
	DelayStatusOnTime  DelayStatus = "ON_TIME"
	DelayStatusEarly   DelayStatus = "EARLY"
)

type ServiceCommitMessage struct {
	Message string                   `json:"message"`
	Type    ServiceCommitMessageType `json:"type"`
}

type ServiceCommitMessageType string

const (
	ServiceCommitMessageTypeBrokerDeliveredDescription                        ServiceCommitMessageType = "BROKER_DELIVERED_DESCRIPTION"
	ServiceCommitMessageTypeCancelledDescription                              ServiceCommitMessageType = "CANCELLED_DESCRIPTION"
	ServiceCommitMessageTypeDeliveryInMultiplePieceShipment                   ServiceCommitMessageType = "DELIVERY_IN_MULTIPLE_PIECE_SHIPMENT"
	ServiceCommitMessageTypeEstimatedDeliveryDateUnavailable                  ServiceCommitMessageType = "ESTIMATED_DELIVERY_DATE_UNAVAILABLE"
	ServiceCommitMessageTypeExceptionInMultiplePieceShipment                  ServiceCommitMessageType = "EXCEPTION_IN_MULTIPLE_PIECE_SHIPMENT"
	ServiceCommitMessageTypeFinalDeliveryAttempted                            ServiceCommitMessageType = "FINAL_DELIVERY_ATTEMPTED"
	ServiceCommitMessageTypeFirstDeliveryAttempted                            ServiceCommitMessageType = "FIRST_DELIVERY_ATTEMPTED"
	ServiceCommitMessageTypeHeldPackageAvailableForRecipientPickup            ServiceCommitMessageType = "HELD_PACKAGE_AVAILABLE_FOR_RECIPIENT_PICKUP"
	ServiceCommitMessageTypeHeldPackageAvailableForRecipientPickupWithAddress ServiceCommitMessageType = "HELD_PACKAGE_AVAILABLE_FOR_RECIPIENT_PICKUP_WITH_ADDRESS"
	ServiceCommitMessageTypeHeldPackageNotAvailableForRecipientPickup         ServiceCommitMessageType = "HELD_PACKAGE_NOT_AVAILABLE_FOR_RECIPIENT_PICKUP"
	ServiceCommitMessageTypeShipmentLabelCreated                              ServiceCommitMessageType = "SHIPMENT_LABEL_CREATED"
	ServiceCommitMessageTypeSubsequentDeliveryAttempted                       ServiceCommitMessageType = "SUBSEQUENT_DELIVERY_ATTEMPTED"
	ServiceCommitMessageTypeUSPSDelivered                                     ServiceCommitMessageType = "USPS_DELIVERED"
	ServiceCommitMessageTypeUSPSDelivering                                    ServiceCommitMessageType = "USPS_DELIVERING"
)

type InformationNote struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type ErrorInfo struct {
	Code          string         `json:"code"`
	ParameterList []*envoy.Entry `json:"parameterList"`
	Message       string         `json:"message"`
}

type SpecialHandling struct {
	Type        string `json:"type"`
	PaymentType string `json:"paymentType"`
	Description string `json:"description"`
}

type AvailableImage struct {
	Size ImageSize `json:"size"`
	Type ImageType `json:"type"`
}

type ImageSize string

const (
	ImageSizeSmall ImageSize = "SMALL"
	ImageSizeLarge ImageSize = "LARGE"
)

type ImageType string

const (
	ImageTypeProodOfDelivery ImageType = "PROOF_OF_DELIVERY"
	ImageTypeBillOfLading    ImageType = "BILL_OF_LADING"
)

type DeliveryDetails struct {
	ReceivedByName                    string                             `json:"receivedByName"`
	SignedByName                      string                             `json:"signedByName"`
	DestinationServiceArea            string                             `json:"destinationServiceArea"`
	DestinationServiceAreaDescription string                             `json:"destinationServiceAreaDescription"`
	LocationType                      LocationType                       `json:"locationType"`
	LocationDescription               string                             `json:"locationDescription"`
	ActualDeliveryAddress             *Address                           `json:"actualDeliveryAddress"`
	DeliveryToday                     bool                               `json:"deliveryToday"`
	DeliveryAttempts                  string                             `json:"deliveryAttempts"`
	DeliveryOptionEligibilityDetails  []*deliveryOptionEligibilityDetail `json:"deliveryOptionEligibilityDetails"`
	OfficeOrderDeliveryMethod         string                             `json:"officeOrderDeliveryMethod"`
}

type LocationType string

const (
	LocationTypeReceptionistOrFrontDesk LocationType = "RECEPTIONIST_OR_FRONT_DESK"
	LocationTypeShippingReceiving       LocationType = "SHIPPING_RECEIVING"
	LocationTypeMailroom                LocationType = "MAILROOM"
	LocationTypeResidence               LocationType = "RESIDENCE"
	LocationTypeGuardOrSecurityStation  LocationType = "GUARD_OR_SECURITY_STATION"
	LocationTypeFedexLocation           LocationType = "FEDEX_LOCATION"
	LocationTypeInBondOrCage            LocationType = "IN_BOND_OR_CAGE"
	LocationTypePharmacy                LocationType = "PHARMACY"
	LocationTypeGateHouse               LocationType = "GATE_HOUSE"
	LocationTypeManagerOffice           LocationType = "MANAGER_OFFICE"
	LocationTypeMainOffice              LocationType = "MAIN_OFFICE"
	LocationTypeLeasingOffice           LocationType = "LEASING_OFFICE"
	LocationTypeRentalOffice            LocationType = "RENTAL_OFFICE"
	LocationTypeApartmentOffice         LocationType = "APARTMENT_OFFICE"
	LocationTypeOther                   LocationType = "OTHER"
)

type deliveryOptionEligibilityDetail struct {
	Option      DeliveryEligibilityOption `json:"option"`
	Eligibility string                    `json:"eligibility"`
}

type DeliveryEligibilityOption string

const (
	DeliveryEligibilityOptionDisputeDelivery          DeliveryEligibilityOption = "DISPUTE_DELIVERY"
	DeliveryEligibilityOptionIndirectSignatureRelease DeliveryEligibilityOption = "INDIRECT_SIGNATURE_RELEASE"
	DeliveryEligibilityOptionRedirectToHoldAtLocation DeliveryEligibilityOption = "REDIRECT_TO_HOLD_AT_LOCATION"
	DeliveryEligibilityOptionReroute                  DeliveryEligibilityOption = "REROUTE"
	DeliveryEligibilityOptionReschedule               DeliveryEligibilityOption = "RESCHEDULE"
	DeliveryEligibilityOptionReturnToShipper          DeliveryEligibilityOption = "RETURN_TO_SHIPPER"
	DeliveryEligibilityOptionSupplementAddress        DeliveryEligibilityOption = "SUPPLEMENT_ADDRESS"
)

type ScanEvent struct {
	Date                 LocalDateTime    `json:"date"`
	EventType            EventType        `json:"eventType"`
	EventDescription     string           `json:"eventDescription"`
	ScanLocation         *Address         `json:"scanLocation"`
	LocationId           string           `json:"locationId"`
	LocationType         ScanLocationType `json:"locationType"`
	DerivedStatus        string           `json:"derivedStatus"`
	DerivedStatusCode    string           `json:"derivedStatusCode"`
	ExceptionDescription string           `json:"exceptionDescription"`
	ExceptionCode        string           `json:"exceptionCode"`
	DelayDetail          *DelayDetail     `json:"delayDetail"`
}

type LocalDateTime struct {
	time.Time
}

func (t *LocalDateTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tz, err := time.Parse(time.RFC3339, s)
	if err == nil {
		*t = LocalDateTime{tz}
		return nil
	}

	tt, err := time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		*t = LocalDateTime{tt}
		return nil
	}

	return err
}

type LocalDate struct {
	time.Time
}

func (t *LocalDate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tt, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}

	*t = LocalDate{tt}
	return nil
}

type ScanLocationType string

const (
	ScanLocationTypeAirport                  ScanLocationType = "AIRPORT"
	ScanLocationTypeCustomsBroker            ScanLocationType = "CUSTOMS_BROKER"
	ScanLocationTypeCustomer                 ScanLocationType = "CUSTOMER"
	ScanLocationTypeDeliveryLocation         ScanLocationType = "DELIVERY_LOCATION"
	ScanLocationTypeDestinationAirport       ScanLocationType = "DESTINATION_AIRPORT"
	ScanLocationTypeDropBox                  ScanLocationType = "DROP_BOX"
	ScanLocationTypeDestinationFedexFacility ScanLocationType = "DESTINATION_FEDEX_FACILITY"
	ScanLocationTypeEnroute                  ScanLocationType = "ENROUTE"
	ScanLocationTypeFedexFacility            ScanLocationType = "FEDEX_FACILITY"
	ScanLocationTypeInterlineCarrier         ScanLocationType = "INTERLINE_CARRIER"
	ScanLocationTypeFedexOfficeLocation      ScanLocationType = "FEDEX_OFFICE_LOCATION"
	ScanLocationTypeNonFedexFacility         ScanLocationType = "NON_FEDEX_FACILITY"
	ScanLocationTypeOriginAirport            ScanLocationType = "ORIGIN_AIRPORT"
	ScanLocationTypeOriginFedexFacility      ScanLocationType = "ORIGIN_FEDEX_FACILITY"
	ScanLocationTypePortOfEntry              ScanLocationType = "PORT_OF_ENTRY"
	ScanLocationTypePickupLocation           ScanLocationType = "PICKUP_LOCATION"
	ScanLocationTypePlane                    ScanLocationType = "PLANE"
	ScanLocationTypeSortFacility             ScanLocationType = "SORT_FACILITY"
	ScanLocationTypeShipAndGetLocation       ScanLocationType = "SHIP_AND_GET_LOCATION"
	ScanLocationTypeTurnpoint                ScanLocationType = "TURNPOINT"
	ScanLocationTypeVehicle                  ScanLocationType = "VEHICLE"
)

type DateAndTime struct {
	DateTime string            `json:"dateTime"`
	Type     TrackingEventType `json:"type"`
}

type TrackingEventType string

const (
	TrackingEventTypeActualDelivery            TrackingEventType = "ACTUAL_DELIVERY"
	TrackingEventTypeActualPickup              TrackingEventType = "ACTUAL_PICKUP"
	TrackingEventTypeActualTender              TrackingEventType = "ACTUAL_TENDER"
	TrackingEventTypeAnticipatedTender         TrackingEventType = "ANTICIPATED_TENDER"
	TrackingEventTypeAppointmentDelivery       TrackingEventType = "APPOINTMENT_DELIVERY"
	TrackingEventTypeAttemptedDelivery         TrackingEventType = "ATTEMPTED_DELIVERY"
	TrackingEventTypeCommitment                TrackingEventType = "COMMITMENT"
	TrackingEventTypeEstimatedArrivalAtGateway TrackingEventType = "ESTIMATED_ARRIVAL_AT_GATEWAY"
	TrackingEventTypeEstimatedDelivery         TrackingEventType = "ESTIMATED_DELIVERY"
	TrackingEventTypeEstimatedPickup           TrackingEventType = "ESTIMATED_PICKUP"
	TrackingEventTypeEstimatedReturnToStation  TrackingEventType = "ESTIMATED_RETURN_TO_STATION"
	TrackingEventTypeShip                      TrackingEventType = "SHIP"
	TrackingEventTypeShipmentDataReceived      TrackingEventType = "SHIPMENT_DATA_RECEIVED"
)

type PackageDetails struct {
	PackageDescription    *PackageDescription   `json:"packageDescription"`
	PhysicalPackagingType PhysicalPackagingType `json:"physicalPackagingType"`
	PackageContent        []string              `json:"packageContent"`
	SequenceNumber        string                `json:"sequenceNumber"`
	Count                 string                `json:"count"`
	ContentPieceCount     string                `json:"contentPieceCount"`
	UndeliveredCount      string                `json:"undeliveredCount"`
	WeightAndDimensions   *WeightAndDimensions  `json:"weightAndDimensions"`
	DeclaredValue         envoy.Value           `json:"declaredValue"`
}

type PhysicalPackagingType string

const (
	PhysicalPackagingTypeBag           PhysicalPackagingType = "BAG"
	PhysicalPackagingTypeBarrel        PhysicalPackagingType = "BARREL"
	PhysicalPackagingTypeBasket        PhysicalPackagingType = "BASKET"
	PhysicalPackagingTypeBox           PhysicalPackagingType = "BOX"
	PhysicalPackagingTypeBucket        PhysicalPackagingType = "BUCKET"
	PhysicalPackagingTypeBundle        PhysicalPackagingType = "BUNDLE"
	PhysicalPackagingTypeCage          PhysicalPackagingType = "CAGE"
	PhysicalPackagingTypeCarton        PhysicalPackagingType = "CARTON"
	PhysicalPackagingTypeCase          PhysicalPackagingType = "CASE"
	PhysicalPackagingTypeChest         PhysicalPackagingType = "CHEST"
	PhysicalPackagingTypeContainer     PhysicalPackagingType = "CONTAINER"
	PhysicalPackagingTypeCrate         PhysicalPackagingType = "CRATE"
	PhysicalPackagingTypeCylinder      PhysicalPackagingType = "CYLINDER"
	PhysicalPackagingTypeDrum          PhysicalPackagingType = "DRUM"
	PhysicalPackagingTypeEnvelope      PhysicalPackagingType = "ENVELOPE"
	PhysicalPackagingTypeHamper        PhysicalPackagingType = "HAMPER"
	PhysicalPackagingTypeOther         PhysicalPackagingType = "OTHER"
	PhysicalPackagingTypePackage       PhysicalPackagingType = "PACKAGE"
	PhysicalPackagingTypePail          PhysicalPackagingType = "PAIL"
	PhysicalPackagingTypePallet        PhysicalPackagingType = "PALLET"
	PhysicalPackagingTypeParcel        PhysicalPackagingType = "PARCEL"
	PhysicalPackagingTypePiece         PhysicalPackagingType = "PIECE"
	PhysicalPackagingTypeReel          PhysicalPackagingType = "REEL"
	PhysicalPackagingTypeRoll          PhysicalPackagingType = "ROLL"
	PhysicalPackagingTypeSack          PhysicalPackagingType = "SACK"
	PhysicalPackagingTypeShrinkwrapped PhysicalPackagingType = "SHRINKWRAPPED"
	PhysicalPackagingTypeSkid          PhysicalPackagingType = "SKID"
	PhysicalPackagingTypeTank          PhysicalPackagingType = "TANK"
	PhysicalPackagingTypeTotebin       PhysicalPackagingType = "TOTEBIN"
	PhysicalPackagingTypeTube          PhysicalPackagingType = "TUBE"
	PhysicalPackagingTypeUnit          PhysicalPackagingType = "UNIT"
)

type PackageDescription struct {
	Type        PackageType `json:"type"`
	Description string      `json:"description"`
}

type PackageType string

const (
	PacakgeTypeYourPackaging      PackageType = "YOUR_PACKAGING"
	PackageTypeFedexEnvelope      PackageType = "FEDEX_ENVELOPE"
	PackageTypeFedexBox           PackageType = "FEDEX_BOX"
	PackageTypeFedexSmallBox      PackageType = "FEDEX_SMALL_BOX"
	PackageTyoeFedexMediumBox     PackageType = "FEDEX_MEDIUM_BOX"
	PackageTypeFedexLargeBox      PackageType = "FEDEX_LARGE_BOX"
	PackageTypeFedexExtraLargeBox PackageType = "FEDEX_EXTRA_LARGE_BOX"
	PackageTypeFedex10KgBox       PackageType = "FEDEX_10KG_BOX"
	PackageTypeFedex25KgBox       PackageType = "FEDEX_25KG_BOX"
	PackageTypeFedexPak           PackageType = "FEDEX_PAK"
	PackageTypeFedexTube          PackageType = "FEDEX_TUBE"
)

type WeightAndDimensions struct {
	Weight     []envoy.Dimensioned `json:"weight"`
	Dimensions []envoy.Size        `json:"dimensions"`
}

type Location struct {
	LocationId                string            `json:"locationId"`
	LocationType              FedexLocationType `json:"locationType"`
	LocationContactAndAddress struct {
		Address *Address `json:"address"`
	} `json:"locationContactAndAddress"`
}

type CustomDeliveryOption struct {
	Type                       CustomDeliveryType          `json:"type"`
	Description                string                      `json:"description"`
	Status                     string                      `json:"status"`
	RequestedAppointmentDetail *RequestedAppointmentDetail `json:"requestedAppointmentDetail"`
}

type CustomDeliveryType string

const (
	CustomDeliveryTypeReroute                    CustomDeliveryType = "REROUTE"
	CustomDeliveryTypeAppointment                CustomDeliveryType = "APPOINTMENT"
	CustomDeliveryTypeDateCertain                CustomDeliveryType = "DATE_CERTAIN"
	CustomDeliveryTypeEvening                    CustomDeliveryType = "EVENING"
	CustomDeliveryTypeRedirectToHoldAtLocation   CustomDeliveryType = "REDIRECT_TO_HOLD_AT_LOCATION"
	CustomDeliveryTypeElectronicSignatureRelease CustomDeliveryType = "ELECTRONIC_SIGNATURE_RELEASE"
)

type RequestedAppointmentDetail struct {
	Date   string            `json:"date"`
	Window []*DeliveryWindow `json:"window"`
}

type DeliveryWindow struct {
	Description string `json:"description"`
	Window      struct {
		Begins time.Time `json:"begins"`
		Ends   time.Time `json:"ends"`
	} `json:"window"`
	Type TrackingEventType `json:"type"`
}

type PieceCount struct {
	Count       string                 `json:"count"`
	Description string                 `json:"description"`
	Type        PieceCountLocationType `json:"type"`
}

type PieceCountLocationType string

const (
	PieceCountLocationTypeDestination PieceCountLocationType = "DESTINATION"
	PieceCountLocationTypeOrigin      PieceCountLocationType = "ORIGIN"
)

type Alert struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Token struct {
	Value      string
	Expiration time.Time
}

func (t *Token) IsValid() bool {
	return t.Expiration.After(time.Now())
}

func (t *Token) UnmarshalJSON(data []byte) error {
	var raw struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	expiration := time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)

	t.Value = raw.AccessToken
	t.Expiration = expiration

	return nil
}
