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

const (
	baseURL = "https://apis.fedex.com"
)

type FedexService struct {
	client    *http.Client
	apiKey    string
	apiSecret string
	token     *token
}

// Enforce that FedexService implements the Service interface
var _ envoy.Service = &FedexService{}

func NewFedexService(client *http.Client, apiKey, apiSecret string) *FedexService {
	return &FedexService{
		client:    client,
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func (s *FedexService) refreshToken() error {
	const endpoint = "/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", s.apiKey)
	data.Set("client_secret", s.apiSecret)

	req, err := http.NewRequest("POST", baseURL+endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	res, err := client.Do(req)
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

	var token token
	if err := json.Unmarshal(body, &token); err != nil {
		return err
	}

	s.token = &token
	return nil
}

func (s *FedexService) Track(trackingNumbers []string) ([]envoy.Parcel, error) {
	const endpoint = "/track/v1/trackingnumbers"

	if s.token == nil || !s.token.isValid() {
		if err := s.refreshToken(); err != nil {
			return nil, err
		}
	}

	data := newTrackingRequest(trackingNumbers)
	reqBody, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", baseURL+endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token.value)
	req.Header.Set("x-locale", "en_US")

	// fmt.Printf("%+v\n\n", req)
	res, err := s.client.Do(req)
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

	var trackingRes trackingResponse
	if err := json.Unmarshal(body, &trackingRes); err != nil {
		return nil, err
	}
	// d, _ := json.MarshalIndent(trackingRes, "", "  ")
	// fmt.Println(string(d))

	var parcels []envoy.Parcel
	for _, r := range trackingRes.Output.CompleteTrackResults {
		parcel := envoy.Parcel{
			Carrier:        envoy.CarrierFedEx,
			TrackingNumber: r.TrackingNumer,
			TrackingURL: fmt.Sprintf(
				"https://www.fedex.com/apps/fedextrack/?tracknumbers=%s",
				r.TrackingNumer,
			),
		}

		for _, r := range r.TrackResults {
			if r.ScanEvents == nil || len(r.ScanEvents) == 0 {
				continue
			}
			var lastEvent *scanEvent
			for _, e := range r.ScanEvents {
				if lastEvent == nil || e.Date.Time.After(lastEvent.Date.Time) {
					lastEvent = e
				}
				if e.EventType == "DL" {
					parcel.Delivered = true
				}
				parcel.TrackingEvents = append(parcel.TrackingEvents, envoy.ParcelEvent{
					Timestamp:   e.Date.Time,
					Description: e.EventDescription,
					Location:    e.ScanLocation.String(),
					Type:        e.EventType.ParcelEventType(),
				})
			}
		}

		parcels = append(parcels, parcel)
	}

	return parcels, nil
}

type trackingRequest struct {
	TrackingInfo         []*trackingInfo `json:"trackingInfo"`
	IncludeDetailedScans bool            `json:"includeDetailedScans"`
}

type trackingInfo struct {
	ShipDateBegin      string              `json:"shipDateBegin,omitempty"`
	ShipDateEnd        string              `json:"shipDateEnd,omitempty"`
	TrackingNumberInfo *trackingNumberInfo `json:"trackingNumberInfo"`
}

type trackingNumberInfo struct {
	TrackingNumber         string `json:"trackingNumber"`
	CarrierCode            string `json:"carrierCode,omitempty"`
	TrackingNumberUniqueId string `json:"trackingNumberUniqueId,omitempty"`
}

func newTrackingRequest(trackingNumbers []string) *trackingRequest {
	tr := &trackingRequest{
		IncludeDetailedScans: true,
	}

	for _, tn := range trackingNumbers {
		tr.TrackingInfo = append(tr.TrackingInfo, &trackingInfo{
			// ShipDateBegin: "2021-01-01",
			// ShipDateEnd:   "2021-12-31",
			TrackingNumberInfo: &trackingNumberInfo{
				TrackingNumber: tn,
				// CarrierCode:    "FDXE",
			},
		})
	}

	return tr
}

// https://developer.fedex.com/api/en-us/catalog/track/v1/docs.html#operation/Track%20by%20Tracking%20Number
type trackingResponse struct {
	TransactionId         string          `json:"transactionId"`
	CustomerTransactionId string          `json:"customerTransactionId"`
	Output                *trackingOutput `json:"output"`
}

type trackingOutput struct {
	CompleteTrackResults []*completeTrackResult `json:"completeTrackResults"`
	Alerts               []*alert               `json:"alerts"`
}

type completeTrackResult struct {
	TrackingNumer string          `json:"trackingNumber"`
	TrackResults  []*trackResults `json:"trackResults"`
}

type trackResults struct {
	TrackingNumberInfo          *trackingNumberInfo     `json:"trackingNumberInfo"`
	AdditionalTrackingInfo      *additionalTrackingInfo `json:"additionalTrackingInfo"`
	DistanceToDestination       envoy.Dimensioned       `json:"distanceToDestination"`
	ConsolidationDetail         []*consolidationDetail  `json:"consolidationDetail"`
	MeterNumber                 string                  `json:"meterNumber"`
	ReturnDetail                *returnDetail           `json:"returnDetail"`
	ServiceDetail               *serviceDetail          `json:"serviceDetail"`
	DestinationLocation         *destinationLocation    `json:"destinationLocation"`
	LastStatusDetail            *statusDetail           `json:"lastStatusDetail"`
	ServiceCommitMessage        serviceCommitMessage    `json:"serviceCommitMessage"`
	InformationNotes            []*informationNote      `json:"informationNotes"`
	Error                       *errorInfo              `json:"error"`
	SpecialHandlings            []*specialHandling      `json:"specialHandlings"`
	AvailableImages             []*availableImage       `json:"availableImages"`
	DeliveryDetails             *deliveryDetails        `json:"deliveryDetails"`
	ScanEvents                  []*scanEvent            `json:"scanEvents"`
	DateAndTimes                []*dateAndTime          `json:"dateAndTimes"`
	PackageDetails              *packageDetails         `json:"packageDetails"`
	GoodsClassificationCode     string                  `json:"goodsClassificationCode"`
	HoldAtLocation              *location               `json:"holdAtLocation"`
	CustomDeliveryOptions       []*customDeliveryOption `json:"customDeliveryOptions"`
	EstimatedDeliveryTimeWindow *deliveryWindow         `json:"estimatedDeliveryTimeWindow"`
	PieceCounts                 []*pieceCount           `json:"pieceCounts"`
	OriginLocation              *location               `json:"originLocation"`
	RecipientInformation        struct {
		Address address `json:"address"`
	} `json:"recipientInformation"`
	StandardTransitTimeWindow *deliveryWindow  `json:"standardTransitTimeWindow"`
	ShipmentDetails           *shipmentDetails `json:"shipmentDetails"`
	ReasonDetail              *reasonDetail    `json:"reasonDetail"`
	AvailableNotifications    []string         `json:"availableNotifications"`
	ShipperInformation        struct {
		Address address `json:"address"`
	} `json:"shipperInformation"`
	LastUpdatedDestinationAddress *address `json:"lastUpdatedDestinationAddress"`
}

type shipmentDetails struct {
	Contents               []*shipmentContent  `json:"contents"`
	BeforePossessionStatus bool                `json:"beforePossessionStatus"`
	Weight                 []envoy.Dimensioned `json:"weight"`
	ContentPieceCount      string              `json:"contentPieceCount"`
	SplitShipments         []*splitShipment    `json:"splitShipments"`
}

type splitShipment struct {
	PieceCount        string    `json:"pieceCount"`
	StatusDescription string    `json:"statusDescription"`
	Timestamp         time.Time `json:"timestamp"`
	StatusCode        string    `json:"statusCode"`
}

type shipmentContent struct {
	ItemNumber       string `json:"itemNumber"`
	ReceivedQuantity string `json:"receivedQuantity"`
	Description      string `json:"description"`
	PartNumber       string `json:"partNumber"`
}

type additionalTrackingInfo struct {
	HasAssociatedShipments bool                 `json:"hasAssociatedShipments"`
	Nickname               string               `json:"nickname"`
	PackageIdentifiers     []*packageIdentifier `json:"packageIdentifiers"`
	ShipmentNotes          string               `json:"shipmentNotes"`
}

type packageIdentifier struct {
	Type                   packageIdentifierType `json:"type"`
	Values                 []string              `json:"values"`
	TrackingNumberUniqueId string                `json:"trackingNumberUniqueId"`
}

type packageIdentifierType string

const (
	packageIdentifierTypeBillOfLading                    packageIdentifierType = "BILL_OF_LADING"
	packageIdentifierTypeCodReturnTrackingNumber         packageIdentifierType = "COD_RETURN_TRACKING_NUMBER"
	packageIdentifierTypeCustomerAuthorizationNumber     packageIdentifierType = "CUSTOMER_AUTHORIZATION_NUMBER"
	packageIdentifierTypeCustomerReference               packageIdentifierType = "CUSTOMER_REFERENCE"
	packageIdentifierTypeDepartment                      packageIdentifierType = "DEPARTMENT"
	packageIdentifierTypeDocumentAirwayBill              packageIdentifierType = "DOCUMENT_AIRWAY_BILL"
	packageIdentifierTypeExpressAlternateReference       packageIdentifierType = "EXPRESS_ALTERNATE_REFERENCE"
	packageIdentifierTypeFedexOfficeJobOrderNumber       packageIdentifierType = "FEDEX_OFFICE_JOB_ORDER_NUMBER"
	packageIdentifierTypeFreeFormReference               packageIdentifierType = "FREE_FORM_REFERENCE"
	packageIdentifierTypeGroundInternational             packageIdentifierType = "GROUND_INTERNATIONAL"
	packageIdentifierTypeGroundShipmentID                packageIdentifierType = "GROUND_SHIPMENT_ID"
	packageIdentifierTypeGroupMPS                        packageIdentifierType = "GROUP_MPS"
	packageIdentifierTypeInternationalDistribution       packageIdentifierType = "INTERNATIONAL_DISTRIBUTION"
	packageIdentifierTypeInvoice                         packageIdentifierType = "INVOICE"
	packageIdentifierTypeJobGlobalTrackingNumber         packageIdentifierType = "JOB_GLOBAL_TRACKING_NUMBER"
	packageIdentifierTypeOrderGlobalTrackingNumber       packageIdentifierType = "ORDER_GLOBAL_TRACKING_NUMBER"
	packageIdentifierTypeOrderToPayNumber                packageIdentifierType = "ORDER_TO_PAY_NUMBER"
	packageIdentifierTypeOutboundLinkToReturn            packageIdentifierType = "OUTBOUND_LINK_TO_RETURN"
	packageIdentifierTypePartNumber                      packageIdentifierType = "PART_NUMBER"
	packageIdentifierTypePartnerCarrierNumber            packageIdentifierType = "PARTNER_CARRIER_NUMBER"
	packageIdentifierTypePurchaseOrder                   packageIdentifierType = "PURCHASE_ORDER"
	packageIdentifierTypeRerouteTrackingNumber           packageIdentifierType = "REROUTE_TRACKING_NUMBER"
	packageIdentifierTypeReturnMaterialsAuthorization    packageIdentifierType = "RETURN_MATERIALS_AUTHORIZATION"
	packageIdentifierTypeReturnedToShipperTrackingNumber packageIdentifierType = "RETURNED_TO_SHIPPER_TRACKING_NUMBER"
	packageIdentifierTypeShipperReference                packageIdentifierType = "SHIPPER_REFERENCE"
	packageIdentifierTypeStandardMPS                     packageIdentifierType = "STANDARD_MPS"
	packageIdentifierTypeTrackingControlNumber           packageIdentifierType = "TRACKING_CONTROL_NUMBER"
	packageIdentifierTypeTrackingNumberOrDoorTag         packageIdentifierType = "TRACKING_NUMBER_OR_DOORTAG"
	packageIdentifierTypeTransborderDistribution         packageIdentifierType = "TRANSBORDER_DISTRIBUTION"
	packageIdentifierTypeTransportationControlNumber     packageIdentifierType = "TRANSPORTATION_CONTROL_NUMBER"
	packageIdentifierTypeVirtualConsolidation            packageIdentifierType = "VIRTUAL_CONSOLIDATION"
)

type consolidationDetail struct {
	TimeStamp       time.Time              `json:"timeStamp"`
	ConsolidationId string                 `json:"consolidationId"`
	ReasonDetail    reasonDetail           `json:"reasonDetail"`
	PackageCount    int                    `json:"packageCount"`
	EventType       consolidationEventType `json:"eventType"`
}

type consolidationEventType string

const (
	consolidationEventTypeAdded    consolidationEventType = "ADDED_TO_CONSOLIDATION"
	consolidationEventTypeRemoved  consolidationEventType = "REMOVED_FROM_CONSOLIDATION"
	consolidationEventTypeExcluded consolidationEventType = "EXCLUDED_FROM_CONSOLIDATION"
)

type reasonDetail struct {
	Description string `json:"description"`
	Type        string `json:"type"`
}

type returnDetail struct {
	AuthorizationName string       `json:"authorizationName"`
	ReasonDetail      reasonDetail `json:"reasonDetail"`
}

type serviceDetail struct {
	Description      string      `json:"description"`
	ShortDescription string      `json:"shortDescription"`
	Type             serviceType `json:"type"`
}

type serviceType string

// https://developer.fedex.com/api/en-us/guides/api-reference.html#servicetypes
const (
	serviceTypeFedexInternationalPriorityExpress      serviceType = "FEDEX_INTERNATIONAL_PRIORITY_EXPRESS"
	serviceTypeFedexInternationalFirst                serviceType = "FEDEX_INTERNATIONAL_FIRST"
	serviceTypeFedexInternationalPriority             serviceType = "FEDEX_INTERNATIONAL_PRIORITY"
	serviceTypeFedexInternationalEconomy              serviceType = "INTERNATIONAL_ECONOMY"
	serviceTypeFedexGround                            serviceType = "FEDEX_GROUND"
	serviceTypeFedexFirstOvernight                    serviceType = "FIRST_OVERNIGHT"
	serviceTypeFedexFirstOvernightFreight             serviceType = "FEDEX_FIRST_FREIGHT"
	serviceTypeFedex1DayFreight                       serviceType = "FEDEX_1_DAY_FREIGHT"
	serviceTypeFedex2DayFreight                       serviceType = "FEDEX_2_DAY_FREIGHT"
	serviceTypeFedex3DayFreight                       serviceType = "FEDEX_3_DAY_FREIGHT"
	serviceTypeFedexInternationalPriorityFreight      serviceType = "INTERNATIONAL_PRIORITY_FREIGHT"
	serviceTypeFedexInternationalEconomyFreight       serviceType = "INTERNATIONAL_ECONOMY_FREIGHT"
	serviceTypeFedexInternationalDeferredFreight      serviceType = "FEDEX_INTERNATIONAL_DEFERRED_FREIGHT"
	serviceTypeFedexInternationalPriorityDistribution serviceType = "INTERNATIONAL_PRIORITY_DISTRIBUTION"
	serviceTypeFedexInternationalDistributionFreight  serviceType = "INTERNATIONAL_DISTRIBUTION_FREIGHT"
	serviceTypeInternationalGroundDistribution        serviceType = "INTL_GROUND_DISTRIBUTION"
	serviceTypeFedexHomeDelivery                      serviceType = "GROUND_HOME_DELIVERY"
	serviceTypeFedexGroundEconomy                     serviceType = "SMART_POST"
	serviceTypeFedexPriorityOvernight                 serviceType = "PRIORITY_OVERNIGHT"
	serviceTypeFedexStandardOvernight                 serviceType = "STANDARD_OVERNIGHT"
	serviceTypeFedex2Day                              serviceType = "FEDEX_2_DAY"
	serviceTypeFedex2DayAM                            serviceType = "FEDEX_2_DAY_AM"
	serviceTypeFedexExpressSaver                      serviceType = "FEDEX_EXPRESS_SAVER"
	serviceTypeFedexSameDay                           serviceType = "SAME_DAY"
	serviceTypeFedexSameDayCity                       serviceType = "SAME_DAY_CITY"
)

type destinationLocation struct {
	LocationId                string                     `json:"locationId"`
	LocationContactAndAddress *locationContactAndAddress `json:"locationContactAndAddress"`
	LocationType              fedexLocationType          `json:"locationType"`
}

type locationContactAndAddress struct {
}

type fedexLocationType string

const (
	fedexLocationTypeAuthorizedShipCenter fedexLocationType = "FEDEX_AUTHORIZED_SHIP_CENTER"
	fedexLocationTypeOffice               fedexLocationType = "FEDEX_OFFICE"
	fedexLocationTypeSelfServiceLocation  fedexLocationType = "FEDEX_SELF_SERVICE_LOCATION"
	fedexLocationTypeGroundTerminal       fedexLocationType = "FEDEX_GROUND_TERMINAL"
	fedexLocationTypeOnsite               fedexLocationType = "FEDEX_ONSITE"
	fedexLocationTypeExpressStation       fedexLocationType = "FEDEX_EXPRESS_STATION"
	fedexLocationTypeFacility             fedexLocationType = "FEDEX_FACILITY"
	fedexLocationTypeFreightServiceCenter fedexLocationType = "FEDEX_FREIGHT_SERVICE_CENTER"
	fedexLocationTypeHomeDeliveryStation  fedexLocationType = "FEDEX_HOME_DELIVERY_STATION"
	fedexLocationTypeShipAndGet           fedexLocationType = "FEDEX_SHIP_AND_GET"
	fedexLocationTypeShipsite             fedexLocationType = "FEDEX_SHIPSITE"
	fedexLocationTypeSmartPostHub         fedexLocationType = "FEDEX_SMART_POST_HUB"
)

type statusDetail struct {
	ScanLocation     *address           `json:"scanLocation"`
	Code             string             `json:"code"`
	DerivedCode      string             `json:"derivedCode"`
	AncillaryDetails []*ancillaryDetail `json:"ancillaryDetails"`
	StatusByLocale   string             `json:"statusByLocale"`
	Description      string             `json:"description"`
	DelayDetail      *delayDetail       `json:"delayDetail"`
}

type eventType string

func (d *eventType) ParcelEventType() envoy.ParcelEventType {
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

type address struct {
	AddressClassification string   `json:"addressClassification"`
	Residential           bool     `json:"residential"`
	StreetLines           []string `json:"streetLines"`
	City                  string   `json:"city"`
	StateOrProvinceCode   string   `json:"stateOrProvinceCode"`
	PostalCode            string   `json:"postalCode"`
	CountryCode           string   `json:"countryCode"`
	CountryName           string   `json:"countryName"`
}

func (a *address) String() string {
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

type ancillaryDetail struct {
	Reason            string `json:"reason"`
	ReasonDesctiption string `json:"reasonDescription"`
	Action            string `json:"action"`
	ActionDescription string `json:"actionDescription"`
}

type delayDetail struct {
	Type    delayType    `json:"type"`
	SubType delaySubType `json:"subType"`
	Status  delayStatus  `json:"status"`
}

type delayType string

const (
	delayTypeWeather     delayType = "WEATHER"
	delayTypeOperational delayType = "OPERATIONAL"
	delayTypeLocal       delayType = "LOCAL"
	delayTypeGeneral     delayType = "GENERAL"
	delayTypeClearance   delayType = "CLEARANCE"
)

type delaySubType string

const (
	delaySubTypeSnow          delaySubType = "SNOW"
	delaySubTypeTornado       delaySubType = "TORNADO"
	delaySubTypeEarthquakeEtc delaySubType = "EARTHQUAKE etc"
)

type delayStatus string

const (
	delayStatusDelayed delayStatus = "DELAYED"
	delayStatusOnTime  delayStatus = "ON_TIME"
	delayStatusEarly   delayStatus = "EARLY"
)

type serviceCommitMessage struct {
	Message string                   `json:"message"`
	Type    serviceCommitMessageType `json:"type"`
}

type serviceCommitMessageType string

const (
	serviceCommitMessageTypeBrokerDeliveredDescription                        serviceCommitMessageType = "BROKER_DELIVERED_DESCRIPTION"
	serviceCommitMessageTypeCancelledDescription                              serviceCommitMessageType = "CANCELLED_DESCRIPTION"
	serviceCommitMessageTypeDeliveryInMultiplePieceShipment                   serviceCommitMessageType = "DELIVERY_IN_MULTIPLE_PIECE_SHIPMENT"
	serviceCommitMessageTypeEstimatedDeliveryDateUnavailable                  serviceCommitMessageType = "ESTIMATED_DELIVERY_DATE_UNAVAILABLE"
	serviceCommitMessageTypeExceptionInMultiplePieceShipment                  serviceCommitMessageType = "EXCEPTION_IN_MULTIPLE_PIECE_SHIPMENT"
	serviceCommitMessageTypeFinalDeliveryAttempted                            serviceCommitMessageType = "FINAL_DELIVERY_ATTEMPTED"
	serviceCommitMessageTypeFirstDeliveryAttempted                            serviceCommitMessageType = "FIRST_DELIVERY_ATTEMPTED"
	serviceCommitMessageTypeHeldPackageAvailableForRecipientPickup            serviceCommitMessageType = "HELD_PACKAGE_AVAILABLE_FOR_RECIPIENT_PICKUP"
	serviceCommitMessageTypeHeldPackageAvailableForRecipientPickupWithAddress serviceCommitMessageType = "HELD_PACKAGE_AVAILABLE_FOR_RECIPIENT_PICKUP_WITH_ADDRESS"
	serviceCommitMessageTypeHeldPackageNotAvailableForRecipientPickup         serviceCommitMessageType = "HELD_PACKAGE_NOT_AVAILABLE_FOR_RECIPIENT_PICKUP"
	serviceCommitMessageTypeShipmentLabelCreated                              serviceCommitMessageType = "SHIPMENT_LABEL_CREATED"
	serviceCommitMessageTypeSubsequentDeliveryAttempted                       serviceCommitMessageType = "SUBSEQUENT_DELIVERY_ATTEMPTED"
	serviceCommitMessageTypeUSPSDelivered                                     serviceCommitMessageType = "USPS_DELIVERED"
	serviceCommitMessageTypeUSPSDelivering                                    serviceCommitMessageType = "USPS_DELIVERING"
)

type informationNote struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type errorInfo struct {
	Code          string         `json:"code"`
	ParameterList []*envoy.Entry `json:"parameterList"`
	Message       string         `json:"message"`
}

type specialHandling struct {
	Description string `json:"description"`
	Type        string `json:"type"`
	PaymentType string `json:"paymentType"`
}

type availableImage struct {
	Size imageSize `json:"size"`
	Type imageType `json:"type"`
}

type imageSize string

const (
	imageSizeSmall imageSize = "SMALL"
	imageSizeLarge imageSize = "LARGE"
)

type imageType string

const (
	imageTypeProodOfDelivery imageType = "PROOF_OF_DELIVERY"
	imageTypeBillOfLading    imageType = "BILL_OF_LADING"
)

type deliveryDetails struct {
	ReceivedByName                    string                             `json:"receivedByName"`
	DestinationServiceArea            string                             `json:"destinationServiceArea"`
	DestinationServiceAreaDescription string                             `json:"destinationServiceAreaDescription"`
	LocationDescription               string                             `json:"locationDescription"`
	ActualDeliveryAddress             *address                           `json:"actualDeliveryAddress"`
	DeliveryToday                     bool                               `json:"deliveryToday"`
	LocationType                      locationType                       `json:"locationType"`
	SignedByName                      string                             `json:"signedByName"`
	OfficeOrderDeliveryMethod         string                             `json:"officeOrderDeliveryMethod"`
	DeliveryAttempts                  string                             `json:"deliveryAttempts"`
	DeliveryOptionEligibilityDetails  []*deliveryOptionEligibilityDetail `json:"deliveryOptionEligibilityDetails"`
}

type locationType string

const (
	locationTypeReceptionistOrFrontDesk locationType = "RECEPTIONIST_OR_FRONT_DESK"
	locationTypeShippingReceiving       locationType = "SHIPPING_RECEIVING"
	locationTypeMailroom                locationType = "MAILROOM"
	locationTypeResidence               locationType = "RESIDENCE"
	locationTypeGuardOrSecurityStation  locationType = "GUARD_OR_SECURITY_STATION"
	locationTypeFedexLocation           locationType = "FEDEX_LOCATION"
	locationTypeInBondOrCage            locationType = "IN_BOND_OR_CAGE"
	locationTypePharmacy                locationType = "PHARMACY"
	locationTypeGateHouse               locationType = "GATE_HOUSE"
	locationTypeManagerOffice           locationType = "MANAGER_OFFICE"
	locationTypeMainOffice              locationType = "MAIN_OFFICE"
	locationTypeLeasingOffice           locationType = "LEASING_OFFICE"
	locationTypeRentalOffice            locationType = "RENTAL_OFFICE"
	locationTypeApartmentOffice         locationType = "APARTMENT_OFFICE"
	locationTypeOther                   locationType = "OTHER"
)

type deliveryOptionEligibilityDetail struct {
	Option      deliveryEligibilityOption `json:"option"`
	Eligibility string                    `json:"eligibility"`
}

type deliveryEligibilityOption string

const (
	deliveryEligibilityOptionDisputeDelivery          deliveryEligibilityOption = "DISPUTE_DELIVERY"
	deliveryEligibilityOptionIndirectSignatureRelease deliveryEligibilityOption = "INDIRECT_SIGNATURE_RELEASE"
	deliveryEligibilityOptionRedirectToHoldAtLocation deliveryEligibilityOption = "REDIRECT_TO_HOLD_AT_LOCATION"
	deliveryEligibilityOptionReroute                  deliveryEligibilityOption = "REROUTE"
	deliveryEligibilityOptionReschedule               deliveryEligibilityOption = "RESCHEDULE"
	deliveryEligibilityOptionReturnToShipper          deliveryEligibilityOption = "RETURN_TO_SHIPPER"
	deliveryEligibilityOptionSupplementAddress        deliveryEligibilityOption = "SUPPLEMENT_ADDRESS"
)

type scanEvent struct {
	Date                 localDateTime    `json:"date"`
	DerivedStatus        string           `json:"derivedStatus"`
	ScanLocation         *address         `json:"scanLocation"`
	LocationId           string           `json:"locationId"`
	LocationType         scanLocationType `json:"locationType"`
	ExceptionDescription string           `json:"exceptionDescription"`
	EventDescription     string           `json:"eventDescription"`
	EventType            eventType        `json:"eventType"`
	DerivedStatusCode    string           `json:"derivedStatusCode"`
	ExceptionCode        string           `json:"exceptionCode"`
	DelayDetail          *delayDetail     `json:"delayDetail"`
}

type localDateTime struct {
	time.Time
}

func (t *localDateTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tz, err := time.Parse(time.RFC3339, s)
	if err == nil {
		*t = localDateTime{tz}
		return nil
	}

	tt, err := time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		*t = localDateTime{tt}
		return nil
	}

	return err
}

type localDate struct {
	time.Time
}

func (t *localDate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	tt, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}

	*t = localDate{tt}
	return nil
}

type scanLocationType string

const (
	scanLocationTypeAirport                  scanLocationType = "AIRPORT"
	scanLocationTypeCustomsBroker            scanLocationType = "CUSTOMS_BROKER"
	scanLocationTypeCustomer                 scanLocationType = "CUSTOMER"
	scanLocationTypeDeliveryLocation         scanLocationType = "DELIVERY_LOCATION"
	scanLocationTypeDestinationAirport       scanLocationType = "DESTINATION_AIRPORT"
	scanLocationTypeDropBox                  scanLocationType = "DROP_BOX"
	scanLocationTypeDestinationFedexFacility scanLocationType = "DESTINATION_FEDEX_FACILITY"
	scanLocationTypeEnroute                  scanLocationType = "ENROUTE"
	scanLocationTypeFedexFacility            scanLocationType = "FEDEX_FACILITY"
	scanLocationTypeInterlineCarrier         scanLocationType = "INTERLINE_CARRIER"
	scanLocationTypeFedexOfficeLocation      scanLocationType = "FEDEX_OFFICE_LOCATION"
	scanLocationTypeNonFedexFacility         scanLocationType = "NON_FEDEX_FACILITY"
	scanLocationTypeOriginAirport            scanLocationType = "ORIGIN_AIRPORT"
	scanLocationTypeOriginFedexFacility      scanLocationType = "ORIGIN_FEDEX_FACILITY"
	scanLocationTypePortOfEntry              scanLocationType = "PORT_OF_ENTRY"
	scanLocationTypePickupLocation           scanLocationType = "PICKUP_LOCATION"
	scanLocationTypePlane                    scanLocationType = "PLANE"
	scanLocationTypeSortFacility             scanLocationType = "SORT_FACILITY"
	scanLocationTypeShipAndGetLocation       scanLocationType = "SHIP_AND_GET_LOCATION"
	scanLocationTypeTurnpoint                scanLocationType = "TURNPOINT"
	scanLocationTypeVehicle                  scanLocationType = "VEHICLE"
)

type dateAndTime struct {
	DateTime string            `json:"dateTime"`
	Type     trackingEventType `json:"type"`
}

type trackingEventType string

const (
	trackingEventTypeActualDelivery            trackingEventType = "ACTUAL_DELIVERY"
	trackingEventTypeActualPickup              trackingEventType = "ACTUAL_PICKUP"
	trackingEventTypeActualTender              trackingEventType = "ACTUAL_TENDER"
	trackingEventTypeAnticipatedTender         trackingEventType = "ANTICIPATED_TENDER"
	trackingEventTypeAppointmentDelivery       trackingEventType = "APPOINTMENT_DELIVERY"
	trackingEventTypeAttemptedDelivery         trackingEventType = "ATTEMPTED_DELIVERY"
	trackingEventTypeCommitment                trackingEventType = "COMMITMENT"
	trackingEventTypeEstimatedArrivalAtGateway trackingEventType = "ESTIMATED_ARRIVAL_AT_GATEWAY"
	trackingEventTypeEstimatedDelivery         trackingEventType = "ESTIMATED_DELIVERY"
	trackingEventTypeEstimatedPickup           trackingEventType = "ESTIMATED_PICKUP"
	trackingEventTypeEstimatedReturnToStation  trackingEventType = "ESTIMATED_RETURN_TO_STATION"
	trackingEventTypeShip                      trackingEventType = "SHIP"
	trackingEventTypeShipmentDataReceived      trackingEventType = "SHIPMENT_DATA_RECEIVED"
)

type packageDetails struct {
	PackageDescription    *packageDescription   `json:"packageDescription"`
	PhysicalPackagingType physicalPackagingType `json:"physicalPackagingType"`
	PackageContent        []string              `json:"packageContent"`
	SequenceNumber        string                `json:"sequenceNumber"`
	Count                 string                `json:"count"`
	ContentPieceCount     string                `json:"contentPieceCount"`
	UndeliveredCount      string                `json:"undeliveredCount"`
	WeightAndDimensions   *weightAndDimensions  `json:"weightAndDimensions"`
	DeclaredValue         envoy.Value           `json:"declaredValue"`
}

type physicalPackagingType string

const (
	physicalPackagingTypeBag           physicalPackagingType = "BAG"
	physicalPackagingTypeBarrel        physicalPackagingType = "BARREL"
	physicalPackagingTypeBasket        physicalPackagingType = "BASKET"
	physicalPackagingTypeBox           physicalPackagingType = "BOX"
	physicalPackagingTypeBucket        physicalPackagingType = "BUCKET"
	physicalPackagingTypeBundle        physicalPackagingType = "BUNDLE"
	physicalPackagingTypeCage          physicalPackagingType = "CAGE"
	physicalPackagingTypeCarton        physicalPackagingType = "CARTON"
	physicalPackagingTypeCase          physicalPackagingType = "CASE"
	physicalPackagingTypeChest         physicalPackagingType = "CHEST"
	physicalPackagingTypeContainer     physicalPackagingType = "CONTAINER"
	physicalPackagingTypeCrate         physicalPackagingType = "CRATE"
	physicalPackagingTypeCylinder      physicalPackagingType = "CYLINDER"
	physicalPackagingTypeDrum          physicalPackagingType = "DRUM"
	physicalPackagingTypeEnvelope      physicalPackagingType = "ENVELOPE"
	physicalPackagingTypeHamper        physicalPackagingType = "HAMPER"
	physicalPackagingTypeOther         physicalPackagingType = "OTHER"
	physicalPackagingTypePackage       physicalPackagingType = "PACKAGE"
	physicalPackagingTypePail          physicalPackagingType = "PAIL"
	physicalPackagingTypePallet        physicalPackagingType = "PALLET"
	physicalPackagingTypeParcel        physicalPackagingType = "PARCEL"
	physicalPackagingTypePiece         physicalPackagingType = "PIECE"
	physicalPackagingTypeReel          physicalPackagingType = "REEL"
	physicalPackagingTypeRoll          physicalPackagingType = "ROLL"
	physicalPackagingTypeSack          physicalPackagingType = "SACK"
	physicalPackagingTypeShrinkwrapped physicalPackagingType = "SHRINKWRAPPED"
	physicalPackagingTypeSkid          physicalPackagingType = "SKID"
	physicalPackagingTypeTank          physicalPackagingType = "TANK"
	physicalPackagingTypeTotebin       physicalPackagingType = "TOTEBIN"
	physicalPackagingTypeTube          physicalPackagingType = "TUBE"
	physicalPackagingTypeUnit          physicalPackagingType = "UNIT"
)

type packageDescription struct {
	Type        packageType `json:"type"`
	Description string      `json:"description"`
}

type packageType string

const (
	pacakgeTypeYourPackaging      packageType = "YOUR_PACKAGING"
	packageTypeFedexEnvelope      packageType = "FEDEX_ENVELOPE"
	packageTypeFedexBox           packageType = "FEDEX_BOX"
	packageTypeFedexSmallBox      packageType = "FEDEX_SMALL_BOX"
	packageTyoeFedexMediumBox     packageType = "FEDEX_MEDIUM_BOX"
	packageTypeFedexLargeBox      packageType = "FEDEX_LARGE_BOX"
	packageTypeFedexExtraLargeBox packageType = "FEDEX_EXTRA_LARGE_BOX"
	packageTypeFedex10KgBox       packageType = "FEDEX_10KG_BOX"
	packageTypeFedex25KgBox       packageType = "FEDEX_25KG_BOX"
	packageTypeFedexPak           packageType = "FEDEX_PAK"
	packageTypeFedexTube          packageType = "FEDEX_TUBE"
)

type weightAndDimensions struct {
	Weight     []envoy.Dimensioned `json:"weight"`
	Dimensions []envoy.Size        `json:"dimensions"`
}

type location struct {
	LocationId                string `json:"locationId"`
	LocationContactAndAddress struct {
		Address address `json:"address"`
	} `json:"locationContactAndAddress"`
	LocationType fedexLocationType `json:"locationType"`
}

type customDeliveryOption struct {
	Type                       customDeliveryType          `json:"type"`
	Description                string                      `json:"description"`
	Status                     string                      `json:"status"`
	RequestedAppointmentDetail *requestedAppointmentDetail `json:"requestedAppointmentDetail"`
}

type customDeliveryType string

const (
	customDeliveryTypeReroute                    customDeliveryType = "REROUTE"
	customDeliveryTypeAppointment                customDeliveryType = "APPOINTMENT"
	customDeliveryTypeDateCertain                customDeliveryType = "DATE_CERTAIN"
	customDeliveryTypeEvening                    customDeliveryType = "EVENING"
	customDeliveryTypeRedirectToHoldAtLocation   customDeliveryType = "REDIRECT_TO_HOLD_AT_LOCATION"
	customDeliveryTypeElectronicSignatureRelease customDeliveryType = "ELECTRONIC_SIGNATURE_RELEASE"
)

type requestedAppointmentDetail struct {
	Date   string            `json:"date"`
	Window []*deliveryWindow `json:"window"`
}

type deliveryWindow struct {
	Description string `json:"description"`
	Window      struct {
		Begins time.Time `json:"begins"`
		Ends   time.Time `json:"ends"`
	} `json:"window"`
	Type trackingEventType `json:"type"`
}

type pieceCount struct {
	Count       string                `json:"count"`
	Description string                `json:"description"`
	Type        piceCountLocationType `json:"type"`
}

type piceCountLocationType string

const (
	piceCountLocationTypeDestination piceCountLocationType = "DESTINATION"
	piceCountLocationTypeOrigin      piceCountLocationType = "ORIGIN"
)

type alert struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type token struct {
	value      string
	expiration time.Time
}

func (t *token) isValid() bool {
	return t.expiration.After(time.Now())
}

func (t *token) UnmarshalJSON(data []byte) error {
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

	t.value = raw.AccessToken
	t.expiration = expiration

	return nil
}
