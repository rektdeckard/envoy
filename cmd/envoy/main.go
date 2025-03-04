package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	envoy "github.com/rektdeckard/envoy/pkg"
	"github.com/rektdeckard/envoy/pkg/fedex"
	"github.com/rektdeckard/envoy/pkg/ups"
	"github.com/rektdeckard/envoy/pkg/usps"
)

var (
	cfg     string
	dbg     bool
	rootCmd = &cobra.Command{
		Use:     "envoy",
		Short:   "Envoy is a command line tool for tracking parcels",
		PreRunE: Init,
		Run:     TUI,
	}
	carrierServices = []envoy.Carrier{
		envoy.CarrierFedEx,
		envoy.CarrierUPS,
		envoy.CarrierUSPS,
	}
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	rootCmd.PersistentFlags().
		StringVarP(
			&cfg,
			"config",
			"c",
			"",
			"Alternate `PATH` to config file",
		)

	rootCmd.PersistentFlags().
		BoolVarP(&dbg, "debug", "d", false, "Enable debug mode")

	for _, c := range carrierServices {
		rootCmd.PersistentFlags().StringSlice(
			strings.ToLower(string(c)),
			[]string{},
			fmt.Sprintf("%s tracking `NUMS` as a comma-separated list", c),
		)
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:        "add",
		Short:      "Adds a new tracking number(s) to the database",
		Args:       cobra.MinimumNArgs(1),
		ArgAliases: []string{"tracking_number"},
		Run:        AddAndRunTUI,
	})
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

func Init(cmd *cobra.Command, args []string) error {
	if err := InitConfig(); err != nil {
		log.Fatal(err)
		return err
	}
	return InitDB(cmd, args)
}

func Add(cmd *cobra.Command, args []string) {

}

func AddAndRunTUI(cmd *cobra.Command, args []string) {

}

func TUI(cmd *cobra.Command, args []string) {
	groups := groupByCarrier(args)
	for _, provider := range []string{"fedex", "ups", "usps"} {
		entries, err := cmd.Flags().GetStringSlice(provider)
		if len(entries) > 0 && err == nil {
			groups[envoy.DetectCarrier(provider)] = append(groups[envoy.DetectCarrier(provider)], entries...)
		}
	}
	runTUI(groups)
}

func syncParcels(args []string) (map[string]*envoy.Parcel, error) {
	groups := groupByCarrier(args)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allParcels := make(map[string]*envoy.Parcel)

	for carrier, trackingNumbers := range groups {
		var svc envoy.Service

		switch carrier {
		case envoy.CarrierFedEx:
			svc = fedex.NewFedexService(
				&http.Client{},
				os.Getenv("FEDEX_API_KEY"),
				os.Getenv("FEDEX_API_SECRET"),
			)
		case envoy.CarrierUPS:
			svc = ups.NewUPSService(
				&http.Client{},
				os.Getenv("UPS_CLIENT_ID"),
				os.Getenv("UPS_CLIENT_SECRET"),
			)
		case envoy.CarrierUSPS:
			svc = usps.NewUSPSService(
				&http.Client{},
				os.Getenv("USPS_CONSUMER_KEY"),
				os.Getenv("USPS_CONSUMER_SECRET"),
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
				return
			}
			for _, p := range parcels {
				if !p.HasData() {
					continue
				}
				if e := p.LastTrackingEvent(); e != nil {
					mu.Lock()
					allParcels[p.TrackingNumber] = p
					mu.Unlock()
					err := UpsertParcel(p)
					if err != nil {
						fmt.Printf("Error upserting parcel %s: %v\n", p.TrackingNumber, err)
					}
				}
			}
		}()
	}

	wg.Wait()
	return allParcels, nil
}

func Track(cmd *cobra.Command, args []string) {
	if err := InitDB(cmd, args); err != nil {
		log.Fatal(err)
	}

	allParcels, err := syncParcels(args)
	if err != nil {
		log.Fatalf("Error syncing parcels: %v", err)
	}

	for id, p := range allParcels {
		if p.HasError() {
			fmt.Printf("%s: %v\n", id, p.Error)
			continue
		}
		fmt.Println(formatEventHistory(p))
	}
}

func groupByCarrier(trackingNumbers []string) map[envoy.Carrier][]string {
	groups := make(map[envoy.Carrier][]string)
	for _, trackingNumber := range trackingNumbers {
		carrier := envoy.DetectCarrier(trackingNumber)
		groups[carrier] = append(groups[carrier], trackingNumber)
	}
	return groups
}
