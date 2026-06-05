package main

import (
	"errors"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func getEnv(e, d string) string {
	value := os.Getenv(e)
	if value == "" {
		value = d
	}
	return value
}

func (api *API) Authenticate(login *Login) (bool, error) {
	if login.Username != api.Username {
		return false, errors.New("Username or password incorrect")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(api.Password), []byte(login.Password)); err != nil {
		log.Printf("%v", err)
		return false, errors.New("Username or password incorrect")
	}
	return true, nil
}

func hash(word string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(word), 8)
	if err != nil {
		panic(err)
	}
	return string(hash)
}
