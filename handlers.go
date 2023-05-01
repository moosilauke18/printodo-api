package main

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/dgrijalva/jwt-go"
	"io"
	"log"
	"net/http"
)

func (api *API) MessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)
	var message Message
	err := decoder.Decode(&message)
	if err != nil {
		var returnString string
		switch {
		case err == io.EOF:
			returnString = "Missing body"
		default:
			log.Println(err)
			returnString = "Bad Request"
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(returnString))
		return
	}
	log.Println(message)
	err = api.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("notes"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}

		id, err := b.NextSequence()
		if err != nil {
			return fmt.Errorf("next sequence: %s", err)
		}
		return b.Put(itob(id), []byte(message.Message))
	})
	if err != nil {
		panic(err)
	}
	w.Write([]byte("OK"))
}

func (api *API) LoginHandler(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var login Login
	w.Header().Set("Content-Type", "application/json")
	err := decoder.Decode(&login)
	if err != nil {
		var returnString string
		switch {
		case err == io.EOF:
			returnString = "Missing body"
		default:
			log.Println(err)
			returnString = "Bad Request"
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(returnString))
		return
	}
	if ok, err := api.Authenticate(&login); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(err.Error()))
		return
	}
	claims := &jwt.StandardClaims{
		Issuer: api.Username,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(api.SigningKey)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(fmt.Sprintf("{\"token\": \"%s\"}", ss)))
}
func (api *API) MessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var n int
	err := api.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("notes"))
		n = b.Stats().KeyN
		return nil
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	messages := make([]string, n)

	err = api.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("notes"))
		count := 0
		err = b.ForEach(func(k, v []byte) error {
			messages[count] = string(v)
			count += 1
			return nil
		})
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}

		return err
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	encoder := json.NewEncoder(w)
	err = encoder.Encode(&messages)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}
func (api *API) ClearMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := api.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("notes"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		b.ForEach(func(k, v []byte) error {
			return b.Delete(k)
		})
		return nil
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("OK"))
}
