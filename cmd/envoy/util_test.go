package main

import (
	"testing"
	"time"

	"github.com/rektdeckard/envoy/pkg"
)

func TestFormatEventOneline(t *testing.T) {
	timeNow := time.Date(2025, 2, 25, 11, 48, 0, 0, time.FixedZone("PST", -8*60*60))

	event := &envoy.ParcelEvent{
		Timestamp:   timeNow,
		Description: "Shipment information sent to FedEx",
		Location:    "Altoona, PA",
	}

	expected := "Tue, Feb 25 2025 11:48 441259201412 Shipment information sent to FedEx @ Altoona, PA"
	result := formatEventOneline("441259201412", event)
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFormatEventHistory(t *testing.T) {
	timeNow := time.Date(2025, 2, 25, 11, 48, 0, 0, time.FixedZone("PST", -8*60*60))

	{
		event1 := &envoy.ParcelEvent{
			Timestamp:   timeNow,
			Description: "Shipment information sent to FedEx",
			Location:    "Altoona, PA",
			Type:        envoy.ParcelEventTypeOrderConfirmed,
		}

		event2 := &envoy.ParcelEvent{
			Timestamp:   timeNow.Add(1 * time.Hour),
			Description: "Package arrived at FedEx location",
			Location:    "Los Angeles, CA",
			Type:        envoy.ParcelEventTypeArrived,
		}

		event3 := &envoy.ParcelEvent{
			Timestamp:   timeNow.Add(26*time.Hour + 36*time.Minute),
			Description: "Delivered",
			Location:    "Los Angeles, CA",
			Type:        envoy.ParcelEventTypeDelivered,
		}

		parcel := &envoy.Parcel{
			Name:           "Test Parcel",
			Carrier:        envoy.CarrierFedEx,
			TrackingNumber: "441259201412",
			Data: &envoy.ParcelData{
				Events:    []envoy.ParcelEvent{*event3, *event2, *event1},
				Delivered: true,
			},
		}

		expected := "✓ Test Parcel (FedEx) DELIVERED\n"
		expected += "└─┬─ • Tue, Feb 25 2025 11:48 Shipment information sent to FedEx @ Altoona, PA"
		expected += "\n  ├─ • Tue, Feb 25 2025 12:48 Package arrived at FedEx location @ Los Angeles, CA"
		expected += "\n  └─ ✓ Wed, Feb 26 2025 14:24 Delivered @ Los Angeles, CA\n"

		result := formatEventHistory(parcel)
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	}

	{
		event1 := &envoy.ParcelEvent{
			Timestamp:   timeNow,
			Description: "Shipment information sent to FedEx",
			Location:    "Altoona, PA",
			Type:        envoy.ParcelEventTypeOrderConfirmed,
		}

		event2 := &envoy.ParcelEvent{
			Timestamp:   timeNow.Add(1 * time.Hour),
			Description: "Package arrived at FedEx location",
			Location:    "Los Angeles, CA",
			Type:        envoy.ParcelEventTypeArrived,
		}

		event3 := &envoy.ParcelEvent{
			Timestamp:   timeNow.Add(26*time.Hour + 36*time.Minute),
			Description: "Delivered",
			Location:    "Los Angeles, CA",
			Type:        envoy.ParcelEventTypeDelivered,
		}

		parcel := &envoy.Parcel{
			Name:           "New shoes",
			Carrier:        envoy.CarrierUPS,
			TrackingNumber: "441259201412",
			Data: &envoy.ParcelData{
				Events:    []envoy.ParcelEvent{*event3, *event2, *event1},
				Delivered: false,
			},
		}

		expected := "✓ New shoes (UPS) DELIVERED\n"
		expected += "└─┬─ • Tue, Feb 25 2025 11:48 Shipment information sent to FedEx @ Altoona, PA"
		expected += "\n  ├─ • Tue, Feb 25 2025 12:48 Package arrived at FedEx location @ Los Angeles, CA"
		expected += "\n  └─ ✓ Wed, Feb 26 2025 14:24 Delivered @ Los Angeles, CA\n"

		result := formatEventHistory(parcel)
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	}
}
