package main

import (
	"log"
	"os"

	"github.com/dgraph-io/badger/v4"
)

func initDb() *badger.DB {
	db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	return db
}
