package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nicetools/backend/internal/config"
	"nicetools/backend/internal/httpapi"
	"nicetools/backend/internal/service"
	"nicetools/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "healthcheck" {
			return errors.New("usage: nicetools [healthcheck]")
		}
		client := http.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://127.0.0.1:" + c.Port + "/api/health")
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		if res.StatusCode != 200 {
			return fmt.Errorf("healthcheck status %d", res.StatusCode)
		}
		return nil
	}
	db, err := store.Open(c)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Sync(c.CatalogPath, c.AdminUsername, c.AdminPassword); err != nil {
		return err
	}
	api := httpapi.New(service.New(db), c)
	if err := api.Err(); err != nil {
		return err
	}
	srv := &http.Server{Addr: ":" + c.Port, Handler: api.Run(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()
	log.Printf("listening on :%s", c.Port)
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-stop.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
