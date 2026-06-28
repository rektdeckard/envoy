package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	envoy "github.com/rektdeckard/envoy/pkg"
	"github.com/rektdeckard/envoy/pkg/fedex"
	"github.com/rektdeckard/envoy/pkg/ups"
	"github.com/rektdeckard/envoy/pkg/usps"
)

const version = "0.1.0"

var (
	conf     Config
	confPath string
	oneline  bool
	rootCmd  = &cobra.Command{
		Use:               "envoy",
		Short:             "Envoy is a command line tool for tracking parcels",
		PersistentPreRunE: initApplication,
		Run:               TUI,
		Version:           version,
	}
	carrierServices = []envoy.Carrier{
		envoy.CarrierFedEx,
		envoy.CarrierUPS,
		envoy.CarrierUSPS,
	}
)

func init() {
	rootCmd.PersistentFlags().
		StringVarP(
			&confPath,
			"config",
			"c",
			"",
			"Alternate `PATH` to config file",
		)
	rootCmd.PersistentFlags().
		StringP("log-level", "l", "warn", "Set log level")

	for _, c := range carrierServices {
		rootCmd.PersistentFlags().StringSlice(
			strings.ToLower(string(c)),
			[]string{},
			fmt.Sprintf("%s tracking `NUMS` as a comma-separated list", c),
		)
	}

	trackCmd := &cobra.Command{
		Use:        "track",
		Short:      "Retrieves the current tracking status for one or more packages",
		SuggestFor: []string{"tracking", "status"},
		Args:       cobra.MinimumNArgs(1),
		ArgAliases: []string{"tracking_number"},
		Run:        Track,
	}
	trackCmd.Flags().BoolVarP(
		&oneline,
		"oneline", "o",
		false,
		"Display tracking information on a single line",
	)

	rootCmd.AddCommand(&cobra.Command{
		Use:        "add",
		Short:      "Adds a new tracking number(s) to the database",
		Args:       cobra.MinimumNArgs(1),
		ArgAliases: []string{"tracking_number"},
		Run:        AddAndRunTUI,
	})
	rootCmd.AddCommand(trackCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("failed to execute", zap.Error(err))
	}
}

func initApplication(cmd *cobra.Command, args []string) error {
	initLogger(cmd)
	conf = initConfig()
	initDB(cmd, args)

	if err := godotenv.Load(); err != nil {
		log.Debugf("could not load .env", zap.Error(err))
	} else {
		log.Debugf("loaded .env", zap.Error(err))
	}

	return nil
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
	log.Debugf("Groups: %+v\n", groups)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allParcels := make(map[string]*envoy.Parcel)

	for carrier, trackingNumbers := range groups {
		var svc envoy.Service

		switch carrier {
		case envoy.CarrierFedEx:
			svc = fedex.NewFedexService(
				&http.Client{},
				conf.Carriers.FedEx.Key,
				conf.Carriers.FedEx.Secret,
			)
		case envoy.CarrierUPS:
			svc = ups.NewUPSService(
				&http.Client{},
				conf.Carriers.UPS.Key,
				conf.Carriers.UPS.Secret,
			)
		case envoy.CarrierUSPS:
			svc = usps.NewUSPSService(
				&http.Client{},
				conf.Carriers.USPS.Key,
				conf.Carriers.USPS.Secret,
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
					err := upsertParcel(p)
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
	initDB(cmd, args)

	allParcels, err := syncParcels(args)
	if err != nil {
		log.Fatalf("Error syncing parcels: %v", err)
	}

	for id, p := range allParcels {
		if p.HasError() {
			fmt.Printf("%s: %v\n", id, p.Error)
			continue
		}
		if oneline {
			fmt.Println(formatEventOneline(p.TrackingNumber, p.LastTrackingEvent()))
		} else {
			fmt.Println(formatEventHistory(p))
		}
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
