package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	envoy "github.com/rektdeckard/envoy/pkg"
)

func RunTUI(groups map[envoy.Carrier][]string) {
	p := tea.NewProgram(model{}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

type view int

const (
	viewParcels view = iota
	viewEvents
)

type model struct {
	parcels          []*envoy.Parcel
	parcelsCursor    int
	parcelsSelection map[int]struct{}
	eventCursor      int
	currentView      view
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "down", "j":
			m.incrementCursor()
		case "up", "k":
			m.decrementCursor()
		case "right", "l":
			m.nextView()
		case "left", "h":
			m.prevView()
		}
	}
	return m, nil
}

func (m model) View() string {
	return "Hello, world!"
}

func (m *model) incrementCursor() {
	count := len(m.parcels)
	if m.currentView == viewParcels {
		if m.parcelsCursor < count-1 {
			m.resetEventCursor()
		}
		m.parcelsCursor = min(count-1, m.parcelsCursor+1)
	} else {
		m.eventCursor = min(count-1, m.eventCursor+1)
	}
}

func (m *model) decrementCursor() {
	if m.currentView == viewParcels {
		if m.parcelsCursor > 0 {
			m.resetEventCursor()
		}
		m.parcelsCursor = max(0, m.parcelsCursor-1)
	} else {
		m.eventCursor = max(0, m.eventCursor-1)
	}
}

func (m *model) nextView() {
	if m.currentView == viewParcels {
		m.currentView = viewEvents
	} else {
		m.currentView = viewParcels
	}
}

func (m *model) prevView() {
	if m.currentView == viewParcels {
		m.currentView = viewEvents
	} else {
		m.currentView = viewParcels
	}
}

func (m *model) resetEventCursor() {
	m.eventCursor = 0
}
