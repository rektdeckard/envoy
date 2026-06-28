package main

import (
	"path"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"

	envoy "github.com/rektdeckard/envoy/pkg"
)

var db *storm.DB

func initDB(_ *cobra.Command, _ []string) {
	dir, err := ConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := path.Join(dir, "envoy.db")

	if db, err = storm.Open(dbPath); err != nil {
		log.Fatal(err)
	}
}

func fetchParcels() ([]*envoy.Parcel, error) {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	var parcels []*envoy.Parcel
	if err := db.All(&parcels); err != nil {
		return nil, err
	}
	return parcels, nil
}

func createParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.Save(p)
}

func updateParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.Update(p)
}

func deleteParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.DeleteStruct(p)
}

func upsertParcels(parcels []*envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	for _, p := range parcels {
		if err := upsertParcel(p); err != nil {
			log.Warnf("Error upserting parcel %s: %v", p.TrackingNumber, err)
			return err
		}
	}
	return nil
}

func upsertParcel(p *envoy.Parcel) error {
	var exists envoy.Parcel
	err := db.One("TrackingNumber", p.TrackingNumber, &exists)

	if err == storm.ErrNotFound {
		return db.Save(p)
	} else if err != nil {
		log.Fatalf("Error checking if parcel %s exists: %v\n", p.TrackingNumber, err)
		return err
	} else {
		return db.Update(p)
	}
}
