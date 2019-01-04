package main

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/dgrijalva/jwt-go"
	"log"
	"net/http"
)

func (api *API) MessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)
	var message Message
	err := decoder.Decode(&message)
	if err != nil {
		// Send reponse
		panic(err)
	}
	log.Println(message)
	err = api.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("notes"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}

		id, _ := b.NextSequence()
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
		log.Println(err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(err.Error()))
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
	}
	w.Write([]byte(fmt.Sprintf("{\"token\": \"%s\"}", ss)))
}
