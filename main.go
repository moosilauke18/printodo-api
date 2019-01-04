package main

import (
	"github.com/boltdb/bolt"
	"github.com/justinas/alice"
	"log"

	"github.com/gorilla/mux"
	"net/http"
)

var (
	JWT_SECRET    []byte
	ENV           string
	isDevelopment bool
)

func main() {

	db, err := bolt.Open("notes.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	api := &API{
		Username:   getEnv("USERNAME", "evan"),
		Password:   hash(getEnv("PASSWORD", "test")),
		Env:        getEnv("ENVIRONMENT", "dev"),
		SigningKey: []byte(getEnv("JWT_SIGNING_KEY", "A53005C9826E1CA34EA6BC1ECEB68E47")),
	}
	JWT_SECRET = api.SigningKey
	if api.Env == "dev" {
		isDevelopment = true
	} else {
		isDevelopment = false
	}
	port := getEnv("PORT", "8000")

	stdMiddleware := alice.New(
		timeoutHandler,
		recoveryHandler,
		rateLimitMiddleware,
	)
	secureMiddleware := stdMiddleware.Append(jwtMiddleware)

	r := mux.NewRouter()
	json := r.Headers("Content-Type", "application/json").Methods("POST").Subrouter()
	json.Handle("/login", stdMiddleware.ThenFunc(api.LoginHandler))
	json.Handle("/message", secureMiddleware.ThenFunc(api.MessageHandler))
	http.Handle("/", r)

	log.Printf("[STARTING] Running server on port: %v", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
