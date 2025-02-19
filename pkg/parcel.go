package envoy

import "time"

type Parcel struct {
	Name           string
	Carrier        Carrier
	TrackingNumber string
	TrackingEvents []ParcelEvent
	TrackingURL    string
	Delivered      bool
}

func (p *Parcel) LastTrackingEvent() *ParcelEvent {
	if len(p.TrackingEvents) == 0 {
		return nil
	}

	var lastEvent *ParcelEvent
	for _, event := range p.TrackingEvents {
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
	ParcelEventTypeOrderConfirmed ParcelEventType = "ORDER CONFIRMED"
	ParcelEventTypeAssertOnTime   ParcelEventType = "EXPECTED ON TIME"
	ParcelEventTypePickedUp       ParcelEventType = "PICKED UP"
	ParcelEventTypeDeparted       ParcelEventType = "DEPARTED"
	ParcelEventTypeProcessing     ParcelEventType = "PROCESSING"
	ParcelEventTypeArrived        ParcelEventType = "ARRIVED"
	ParcelEventTypeOnVehicle      ParcelEventType = "ON DELIVERY VEHICLE"
	ParcelEventTypeOutForDelivery ParcelEventType = "OUT FOR DELIVERY"
	ParcelEventTypeDelivered      ParcelEventType = "DELIVERED"
	ParcelEventTypeUnknown        ParcelEventType = "UNKNOWN"
)
