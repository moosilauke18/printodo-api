package main

import (
	"github.com/boltdb/bolt"
)

type API struct {
	Username   string
	Password   string
	Env        string
	SigningKey []byte
	db         *bolt.DB
}

type Message struct {
	Message string
}

type Login struct {
	Username string `json: "username"`
	Password string `json: "username"`
}
