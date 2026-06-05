package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	JWT_SECRET    []byte
	ENV           string
	isDevelopment bool
)

// openAPI opens the database, runs migrations, and builds the API value shared
// by the server and the backport subcommand.
func openAPI() *API {
	db, err := gorm.Open(postgres.Open(buildDSN()), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&Item{}, &Category{}); err != nil {
		log.Fatal(err)
	}

	api := &API{
		Username:   getEnv("USERNAME", "test"),
		Password:   hash(getEnv("PASSWORD", "test")),
		Env:        getEnv("ENVIRONMENT", "dev"),
		SigningKey: []byte(getEnv("JWT_SIGNING_KEY", "A53005C9826E1CA34EA6BC1ECEB68E47")),
		db:         db,
	}
	JWT_SECRET = api.SigningKey
	isDevelopment = api.Env == "dev"
	return api
}

func main() {

	// Subcommand: backport historical notes from a file, then exit. Handled
	// before opening the DB so `--dry-run` needs no database connection.
	if len(os.Args) > 1 && os.Args[1] == "backport" {
		runBackportCLI(os.Args[2:])
		return
	}

	// Subcommand: re-run AI classification over existing items, then exit.
	if len(os.Args) > 1 && os.Args[1] == "reclassify" {
		runReclassifyCLI(os.Args[2:])
		return
	}

	api := openAPI()
	port := getEnv("PORT", "8000")

	// One-time import of any legacy BoltDB data into Postgres.
	api.importBoltData("notes.db")

	stdMiddleware := alice.New(
		timeoutHandler,
		recoveryHandler,
	)
	unsecureMiddleware := stdMiddleware.Append(rateLimitMiddleware)
	secureMiddleware := stdMiddleware.Append(jwtMiddleware)
	// The browser admin site needs more headroom than the 1s API timeout and
	// uses cookie auth instead of the header JWT middleware.
	adminMiddleware := alice.New(adminTimeoutHandler, recoveryHandler)

	r := mux.NewRouter()

	// Browser-facing admin website (cookie auth).
	r.Handle("/admin/login", adminMiddleware.ThenFunc(api.handleAdminLogin)).Methods("GET", "POST")
	r.Handle("/admin/logout", adminMiddleware.ThenFunc(api.handleAdminLogout)).Methods("GET")
	r.Handle("/admin", adminMiddleware.ThenFunc(api.adminAuth(api.handleAdminDashboard))).Methods("GET")
	r.Handle("/admin/data", adminMiddleware.ThenFunc(api.adminAuth(api.handleAdminData))).Methods("GET")
	r.Handle("/admin/items/{id}/classify", adminMiddleware.ThenFunc(api.adminAuth(api.handleAdminClassify))).Methods("POST")

	// JSON API for the mobile app and the printer worker (unchanged contracts).
	client := r.Headers("Content-Type", "application/json").Methods("POST").Subrouter()
	printer := r.Headers("User-Agent", "todo-printer/1.0").Headers("Content-Type", "application/json").Subrouter()
	client.Handle("/login", unsecureMiddleware.ThenFunc(api.LoginHandler))
	client.Handle("/message", secureMiddleware.ThenFunc(api.MessageHandler))
	printer.Handle("/messages", secureMiddleware.ThenFunc(api.MessagesHandler)).Methods("GET")
	printer.Handle("/messages", secureMiddleware.ThenFunc(api.ClearMessagesHandler)).Methods("DELETE")

	log.Printf("[STARTING] Running server on port: %v", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
