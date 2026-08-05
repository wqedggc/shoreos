package main

import (
	"log"
	"net/http"
	"time"

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

	server, err := api.NewServer(store, cfg)
	if err != nil {
		log.Fatalf("new server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("ShoreOS FIRE listening on %s", cfg.HTTPAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
