// Command server runs the rockets HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/httpapi"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

func main() {
	addr := flag.String("addr", ":8088", "address to listen on")
	dbPath := flag.String("db", "rockets.db", "SQLite database file")
	reset := flag.Bool("reset", false, "delete all stored state before starting")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := storage.OpenSQLite(*dbPath)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if *reset {
		if err := db.Reset(context.Background()); err != nil {
			log.Error("reset database", "error", err)
			os.Exit(1)
		}
		log.Info("database reset", "path", *dbPath)
	}

	svc := rocket.NewService(db)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(svc, log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "error", err)
	}
}
