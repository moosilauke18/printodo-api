package main

import (
	"fmt"
	"os"
)

// buildDSN mirrors the convention used by the sibling "upkeep" app so printodo
// can point at the same Postgres server. It prefers individual PG_* vars
// (shared K8s secrets) and falls back to DATABASE_URL for local dev. The only
// difference from upkeep is the default database name, which is "printodo".
func buildDSN() string {
	if pgHost := os.Getenv("PG_HOST"); pgHost != "" {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
			pgHost,
			getEnv("PG_PORT", "5432"),
			getEnv("PG_USER", "postgres"),
			os.Getenv("PG_PASSWORD"),
			getEnv("PG_DB", "printodo"),
		)
	}
	return getEnv("DATABASE_URL", "postgres://printodo:printodo@localhost:5432/printodo?sslmode=disable")
}
