package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rektdeckard/envoy/pkg"
)

func prepend[T any](s []T, v T) []T {
	return append([]T{v}, s...)
}

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))

	iconDefault   = "•"
	iconDelivered = successStyle.Inline(true).Render("✓")
	iconException = errorStyle.Inline(true).Render("✗")

	ldr = dimStyle.Render("└─┬─")
	lvn = dimStyle.Render("  │ ")
	lvr = dimStyle.Render("  ├─")
	lur = dimStyle.Render("  └─")
	lor = dimStyle.Render("└───")
)

func formatEventIcon(e *envoy.ParcelEvent) string {
	switch e.Type {
	case envoy.ParcelEventTypeDelivered:
		return iconDelivered
	case envoy.ParcelEventTypeUnknown:
		return iconException
	default:
		return iconDefault
	}
}

// Format an event as a single line of text in the format:
// Tue, 25 Feb 2025 11:48:00 -0800 441259201412 Shipment information sent to FedEx
func formatEventOneline(nameOrTrackingNumber string, e *envoy.ParcelEvent) string {
	name := nameOrTrackingNumber
	if name != "" {
		name = " " + name
	}

	return fmt.Sprintf(
		"%s%s %s @ %s",
		e.Timestamp.Format(timeFormat),
		name,
		e.Description,
		e.Location,
	)
}

// Format the event history for a parcel as a timeline of events
func formatEventHistory(parcel *envoy.Parcel) string {

	if !parcel.HasData() {
		return ""
	}

	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf(
		"%s %s (%s) %s\n",
		formatEventIcon(parcel.LastTrackingEvent()),
		parcel.Name,
		parcel.Carrier,
		parcel.LastTrackingEvent().Type,
	))
	ct := len(parcel.Data.Events)
	for i := range ct {
		e := parcel.Data.Events[ct-i-1]
		prefix := lvr
		if ct == 1 {
			prefix = lor
		} else if i == 0 {
			prefix = ldr
		} else if i == len(parcel.Data.Events)-1 {
			prefix = lur
		}
		sb.WriteString(fmt.Sprintf(
			"%s %s %s\n",
			prefix,
			formatEventIcon(&e),
			formatEventOneline("", &e),
		))
	}
	return sb.String()
}
