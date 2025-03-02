package envoy

import "time"

type Parcel struct {
	Name           string  `storm:"index"`
	Carrier        Carrier `storm:"index"`
	TrackingNumber string  `storm:"id"`
	TrackingURL    string
	Data           *ParcelData
	Error          error
}

type ParcelData struct {
	Events             []ParcelEvent
	Delivered          bool
	DeliveryProjection *time.Time
}

func NewParcel(name string, carrier Carrier, trackingNumber, trackingURL string) *Parcel {
	return &Parcel{
		Name:           name,
		Carrier:        carrier,
		TrackingNumber: trackingNumber,
		TrackingURL:    trackingURL,
	}
}

func (p *Parcel) HasData() bool {
	return p.Data != nil
}

func (p *Parcel) HasError() bool {
	return p.Error != nil
}

func (p *Parcel) LastTrackingEvent() *ParcelEvent {
	if !p.HasData() {
		return nil
	}
	if len(p.Data.Events) == 0 {
		return nil
	}

	var lastEvent *ParcelEvent
	for _, event := range p.Data.Events {
		if lastEvent == nil || event.Timestamp.After(lastEvent.Timestamp) {
			lastEvent = &event
		}
	}
	return lastEvent
}

type ParcelEvent struct {
	Type        ParcelEventType
	Description string
	Location    string
	Timestamp   time.Time
}

type ParcelEventType string

const (
	ParcelEventTypeOrderConfirmed         ParcelEventType = "ORDER CONFIRMED"
	ParcelEventTypeAssertOnTime           ParcelEventType = "EXPECTED ON TIME"
	ParcelEventTypePickedUp               ParcelEventType = "PICKED UP"
	ParcelEventTypeDeparted               ParcelEventType = "DEPARTED"
	ParcelEventTypeProcessing             ParcelEventType = "PROCESSING"
	ParcelEventTypeArrived                ParcelEventType = "ARRIVED"
	ParcelEventTypeOnVehicle              ParcelEventType = "ON DELIVERY VEHICLE"
	ParcelEventTypeOutForDelivery         ParcelEventType = "OUT FOR DELIVERY"
	ParcelEventTypeDelivered              ParcelEventType = "DELIVERED"
	ParcelEventTypeDelayed                ParcelEventType = "DELAYED"
	ParcelEventTypeParcelHeld             ParcelEventType = "HELD"
	ParcelEventTypeAwaitingCustomerAction ParcelEventType = "AWAITING CUSTOMER ACTION"
	ParcelEventTypeAwaitingCustomerPickup ParcelEventType = "AWAITING CUSTOMER PICKUP"
	ParcelEventTypeTransferredToLocal     ParcelEventType = "TRANSFERRED TO LOCAL"
	ParcelEventTypeUndeliverable          ParcelEventType = "UNDELIVERABLE"
	ParcelEventTypeReturnedToSender       ParcelEventType = "RETURNED TO SENDER"
	ParcelEventTypeUnknown                ParcelEventType = "UNKNOWN"
)
