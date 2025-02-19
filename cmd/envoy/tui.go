package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/skratchdot/open-golang/open"

	"github.com/rektdeckard/envoy/pkg"
	"github.com/rektdeckard/envoy/pkg/fedex"
	"github.com/rektdeckard/envoy/pkg/ups"
)

const (
	timeFormat = "Mon, Jan 02 2006 15:04"
)

var (
	baseStyle                    = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder())
	successStyle                 = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	errorStyle                   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
	tableWithActiveSelectedStyle = func() table.Styles {
		s := table.DefaultStyles()
		s.Header = s.Header.
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true)
		s.Selected = s.Selected.
			Foreground(lipgloss.ANSIColor(0)).
			Background(lipgloss.ANSIColor(3))
		return s
	}()
	tableWithInctiveSelectedStyle = func() table.Styles {
		s := tableWithActiveSelectedStyle
		s.Selected = s.Selected.
			Foreground(lipgloss.ANSIColor(7)).
			Background(lipgloss.ANSIColor(8))
		return s
	}()
)

func runTUI(groups map[envoy.Carrier][]string) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	wg := sync.WaitGroup{}
	allParcels := make(map[string]*envoy.Parcel)

	for carrier, trackingNumbers := range groups {
		var svc envoy.Service

		switch carrier {
		case envoy.CarrierFedEx:
			svc = fedex.NewFedexService(
				&client,
				os.Getenv("FEDEX_API_KEY"),
				os.Getenv("FEDEX_API_SECRET"),
			)
		case envoy.CarrierUPS:
			svc = ups.NewUPSService(
				&http.Client{},
				os.Getenv("UPS_CLIENT_ID"),
				os.Getenv("UPS_CLIENT_SECRET"),
			)

		default:
			fmt.Printf("Unsupported carrier: %v\n", carrier)
			os.Exit(1)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			parcels, err := svc.Track(trackingNumbers)
			if err != nil {
				fmt.Printf("Err: %+v\n", err)
			}
			for _, p := range parcels {
				if e := p.LastTrackingEvent(); e != nil {
					allParcels[p.TrackingNumber] = &p
				}
			}
		}()
	}

	wg.Wait()

	columns := []table.Column{
		{Title: "PARCEL NAME", Width: 16},
		{Title: "CARRIER", Width: 8},
		{Title: "TRACKING NO.", Width: 16},
		{Title: "STATUS", Width: 16},
		{Title: "DATE", Width: 28},
	}

	var rows []table.Row
	for _, p := range allParcels {
		if p.Name == "" {
			p.Name = p.TrackingNumber
		}
		name := p.Name
		status := strings.ToUpper(p.LastTrackingEvent().Description)
		// if p.Delivered {
		// 	status = successStyle.Render(status)
		// }
		rows = append(rows, table.Row{
			name,
			string(p.Carrier),
			p.TrackingNumber,
			status,
			p.LastTrackingEvent().Timestamp.Format(timeFormat),
		})
	}

	t := table.New(
		table.WithStyles(tableWithActiveSelectedStyle),
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	eColumns := []table.Column{
		{Title: "EVENT", Width: 16},
		{Title: "LOCATION", Width: 16},
		{Title: "DATE", Width: 24},
		{Title: "NOTES", Width: 30},
	}
	var eRows []table.Row
	keys := make([]string, 0, len(allParcels))
	for k := range allParcels {
		keys = append(keys, k)
	}
	if len(keys) > 0 {
		for _, p := range allParcels[keys[0]].TrackingEvents {
			eRows = append(eRows, table.Row{
				string(p.Type),
				p.Location,
				p.Timestamp.Format(timeFormat),
				p.Description,
			})
		}
	}

	s2 := tableWithInctiveSelectedStyle
	t2 := table.New(
		table.WithStyles(s2),
		table.WithColumns(eColumns),
		table.WithRows(eRows),
		table.WithFocused(false),
		table.WithHeight(9),
	)

	zone.NewGlobal()

	p := tea.NewProgram(
		model{
			parcels:      allParcels,
			parcelsTable: t,
			eventsTable:  t2,
		},
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
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
	parcels          map[string]*envoy.Parcel
	parcelsSelection map[int]struct{}
	currentView      view
	parcelsTable     table.Model
	eventsTable      table.Model
}

func (m model) Init() tea.Cmd {
	m.parcelsTable.Focus()
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	m.parcelsTable, cmd = m.parcelsTable.Update(msg)
	cmds = append(cmds, cmd)

	m.eventsTable, cmd = m.eventsTable.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w, h := baseStyle.GetFrameSize()

		m.parcelsTable.SetWidth(msg.Width - w - 2)
		cols := m.parcelsTable.Columns()
		cols[len(cols)-1].Width = msg.Width - w - 2 - 66
		m.parcelsTable.SetColumns(cols)

		m.eventsTable.SetWidth(msg.Width - w - 2)
		cols = m.eventsTable.Columns()
		cols[len(cols)-1].Width = msg.Width - w - 2 - 64
		m.eventsTable.SetColumns(cols)
		m.eventsTable.SetHeight(msg.Height - (2 * h) - m.parcelsTable.Height() - 7)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			cmd := m.toggleView()
			cmds = append(cmds, cmd)
		case "enter":
			cmd := m.setEventsView()
			cmds = append(cmds, cmd)
		case "esc":
			cmd := m.setParcelsView()
			cmds = append(cmds, cmd)
		case "o":
			if s := m.parcelsTable.SelectedRow(); s != nil {
				parcel := m.parcels[s[2]]
				open.Run(parcel.TrackingURL)
			}
		}

		if len(m.parcels) > 0 && key.Matches(msg,
			m.parcelsTable.KeyMap.LineUp,
			m.parcelsTable.KeyMap.LineDown,
			m.parcelsTable.KeyMap.PageUp,
			m.parcelsTable.KeyMap.PageDown,
			m.parcelsTable.KeyMap.HalfPageUp,
			m.parcelsTable.KeyMap.HalfPageDown,
			m.parcelsTable.KeyMap.GotoTop,
			m.parcelsTable.KeyMap.GotoBottom,
		) {
			parcel := m.parcels[m.parcelsTable.SelectedRow()[2]]

			var eRows []table.Row
			for _, p := range parcel.TrackingEvents {
				eRows = append(eRows, table.Row{
					string(p.Type),
					p.Location,
					p.Timestamp.Format(timeFormat),
					p.Description,
				})
			}
			m.eventsTable.SetRows(eRows)
		}

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
	default:
		panic(fmt.Sprintf("%+v", msg))
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	view := lipgloss.JoinVertical(
		lipgloss.Left,
		zone.Mark("parcels", baseStyle.Render(m.parcelsTable.View())),
		zone.Mark("events", baseStyle.Render(m.eventsTable.View())),
		m.eventsTable.HelpView(),
	)
	return zone.Scan(view)
}

func initialModel() model {
	return model{
		currentView: viewParcels,
	}
}

func (m *model) toggleView() tea.Cmd {
	if m.currentView == viewParcels {
		return m.setEventsView()
	}
	return m.setParcelsView()
}

func (m *model) setParcelsView() tea.Cmd {
	m.currentView = viewParcels
	m.parcelsTable.SetStyles(tableWithActiveSelectedStyle)
	m.eventsTable.SetStyles(tableWithInctiveSelectedStyle)
	m.eventsTable.Blur()
	m.parcelsTable.Focus()
	return nil
}

func (m *model) setEventsView() tea.Cmd {
	m.currentView = viewEvents
	m.eventsTable.SetStyles(tableWithActiveSelectedStyle)
	m.parcelsTable.SetStyles(tableWithInctiveSelectedStyle)
	m.parcelsTable.Blur()
	m.eventsTable.Focus()
	return nil
}
