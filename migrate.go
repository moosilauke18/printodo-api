package main

import (
	"log"
	"os"
	"time"

	"github.com/boltdb/bolt"
)

// importBoltData is a one-time migration: if a legacy BoltDB file exists, read
// every note out of its "notes" bucket and insert each as an unprinted Item,
// then rename the file so this never runs again. Bolt stored no timestamps, so
// imported items are stamped with the import time.
//
// It is safe to call on every startup: when notes.db is absent (the normal
// case after the first run), it does nothing.
func (api *API) importBoltData(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		log.Printf("[migrate] could not open %s, skipping import: %v", path, err)
		return
	}

	var texts []string
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("notes"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			texts = append(texts, string(v))
			return nil
		})
	})
	db.Close()
	if err != nil {
		log.Printf("[migrate] could not read notes from %s: %v", path, err)
		return
	}

	now := time.Now()
	imported := 0
	for _, t := range texts {
		item := &Item{Text: t, CreatedAt: now}
		if err := api.db.Create(item).Error; err != nil {
			log.Printf("[migrate] failed to import a note: %v", err)
			continue
		}
		imported++
	}

	// Rename so the import won't repeat on the next startup.
	archived := path + ".imported"
	if err := os.Rename(path, archived); err != nil {
		log.Printf("[migrate] imported %d notes but could not rename %s (may re-import next start): %v", imported, path, err)
		return
	}
	log.Printf("[migrate] imported %d notes from %s into Postgres; archived file as %s", imported, path, archived)
}
