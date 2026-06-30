package main

import (
	"log"
	"net/http"

	"github.com/wqedggc/shoreos/internal/api"
	"github.com/wqedggc/shoreos/internal/config"
	storemysql "github.com/wqedggc/shoreos/internal/repository/mysql"
)

func main() {
	cfg := config.Load()
	store, err := storemysql.Open(cfg)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer store.Close()

	server, err := api.NewServer(store)
	if err != nil {
		log.Fatalf("new server: %v", err)
	}

	log.Printf("ShoreOS FIRE listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
