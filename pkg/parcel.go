package service

import "time"

type Parcel struct {
	TrackingNumber string
	TrackingEvents []ParcelEvent
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

type ParcelEventType int

const (
	ParcelEventTypeOrderConfirmed ParcelEventType = iota
	ParcelEventTypeAssertOnTime
	ParcelEventTypePickedUp
	ParcelEventTypeDeparted
	ParcelEventTypeArrived
	ParcelEventTypeOutForDelivery
	ParcelEventTypeDelivered
	ParcelEventTypeUnknown
)
