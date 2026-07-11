// Command chronos is a single-binary time & client-work tracker. It serves a
// JSON API and an embedded web UI from one port, backed by a local SQLite file.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"chronos/internal/api"
	"chronos/internal/config"
	"chronos/internal/store"
	"chronos/internal/webui"
)

func main() {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database %q: %v", cfg.DBPath, err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	// Attachment bytes live next to the database in chronos-files/.
	filesDir := filepath.Join(filepath.Dir(cfg.DBPath), "chronos-files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		log.Fatalf("create files dir %q: %v", filesDir, err)
	}

	mux := api.New(st, filesDir).Routes()
	// Everything not matched by an /api route falls through to the UI.
	mux.Handle("/", webui.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("chronos listening on %s (db: %s)", cfg.Addr, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on Ctrl-C / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Print("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
