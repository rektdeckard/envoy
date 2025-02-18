package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	envoy "github.com/rektdeckard/envoy/pkg"
	"github.com/rektdeckard/envoy/pkg/fedex"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "envoy",
		Short: "Envoy is a command line tool for tracking parcels",
		Run:   TUI,
	}
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading .env file")
		os.Exit(1)
	}

	rootCmd.PersistentFlags().
		StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra.yaml)")

	rootCmd.AddCommand(&cobra.Command{
		Use:        "track",
		Short:      "Retrieves the current tracking status for one or more packages",
		SuggestFor: []string{"tracking", "status"},
		Args:       cobra.MinimumNArgs(1),
		ArgAliases: []string{"tracking_number"},
		Run:        Track,
	})

	rootCmd.Execute()
}

func TUI(cmd *cobra.Command, args []string) {
	// TODO: use saved tracking numbers from DB
	groups := groupByCarrier([]string{
		"271278612814",
		"281958973124",
		"271198840120",
		"271245206460",
		"271163815798",
	})
	runTUI(groups)
}

func Track(cmd *cobra.Command, args []string) {
	groups := groupByCarrier(args)

	var wg sync.WaitGroup

	for carrier, trackingNumbers := range groups {
		var svc envoy.Service

		switch carrier {
		case envoy.CarrierFedEx:
			svc = fedex.NewFedexService(
				&http.Client{},
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
