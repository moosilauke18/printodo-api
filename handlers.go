package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dgrijalva/jwt-go"
)

// MessageHandler is hit by the mobile app (POST /message). It records the item
// as history (which also enqueues it for the printer) and kicks off an
// asynchronous AI category suggestion. Response contract is unchanged ("OK").
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

	item, err := api.CreateItem(message.Message)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to store message"))
		return
	}

	// Suggest categories in the background so it's ready by the time the user
	// visits the admin site, without slowing down the print path.
	go api.suggestCategories(item.ID, item.Text)

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

// MessagesHandler is hit by the printer worker (GET /messages). It returns the
// text of every not-yet-printed item as a JSON array of strings — unchanged
// contract.
func (api *API) MessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	messages, err := api.PendingTexts()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(&messages); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// ClearMessagesHandler is hit by the printer worker (DELETE /messages) after it
// prints. It clears the queue by marking pending items printed, preserving them
// as browsable history rather than deleting — unchanged contract ("OK").
func (api *API) ClearMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := api.MarkPendingPrinted(); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte("OK"))
}
