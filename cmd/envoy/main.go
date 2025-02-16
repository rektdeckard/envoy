package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	envoy "github.com/rektdeckard/envoy/pkg"
	"github.com/rektdeckard/envoy/pkg/fedex"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading .env file")
		os.Exit(1)
	}
	cmd := &cobra.Command{
		Use:   "envoy",
		Short: "Envoy is a command line tool for tracking parcels",
		Run:   tui,
	}

	cmd.AddCommand(&cobra.Command{
		Use:        "track",
		Short:      "Retrieves the current tracking status for one or more packages",
		SuggestFor: []string{"tracking", "status"},
		Args:       cobra.MinimumNArgs(1),
		ArgAliases: []string{"tracking_number"},
		Run:        track,
	})

	cmd.Execute()
}

func tui(cmd *cobra.Command, args []string) {
	groups := groupByCarrier(args)
	RunTUI(groups)
}

func track(cmd *cobra.Command, args []string) {
	groups := groupByCarrier(args)
	fmt.Printf("%+v\n", groups)

	var wg sync.WaitGroup

	for carrier, trackingNumbers := range groups {
		var svc envoy.Service

		switch carrier {
		case envoy.CarrierFedEx:
			svc = fedex.NewFedexService(
				os.Getenv("FEDEX_API_KEY"),
				os.Getenv("FEDEX_API_SECRET"),
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
					fmt.Printf("%s %s %s\n", e.Timestamp.Format(time.RFC1123), p.TrackingNumber, e.Description)
				}
			}
		}()
	}

	wg.Wait()
}

func groupByCarrier(trackingNumbers []string) map[envoy.Carrier][]string {
	groups := make(map[envoy.Carrier][]string)
	for _, trackingNumber := range trackingNumbers {
		carrier := envoy.DetectCarrier(trackingNumber)
		groups[carrier] = append(groups[carrier], trackingNumber)
	}
	return groups
}
