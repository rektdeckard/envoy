package main

import (
	"log"
	"path"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"

	envoy "github.com/rektdeckard/envoy/pkg"
)

var db *storm.DB

func InitDB(cmd *cobra.Command, args []string) error {
	dir, err := ConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := path.Join(dir, "envoy.db")

	if db, err = storm.Open(dbPath); err != nil {
		log.Fatal(err)
	}
	return nil
}

func FetchParcels() ([]*envoy.Parcel, error) {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	var parcels []*envoy.Parcel
	if err := db.All(&parcels); err != nil {
		return nil, err
	}
	return parcels, nil
}

func CreateParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.Save(p)
}

func UpdateParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.Update(p)
}

func DeleteParcel(p *envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	return db.DeleteStruct(p)
}

func UpsertParcels(parcels []*envoy.Parcel) error {
	if db == nil {
		log.Fatal("Error:  DB is not initialized")
	}
	for _, p := range parcels {
		if err := UpsertParcel(p); err != nil {
			log.Printf("Error upserting parcel %s: %v", p.TrackingNumber, err)
			return err
		}
	}
	return nil
}

func UpsertParcel(p *envoy.Parcel) error {
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
