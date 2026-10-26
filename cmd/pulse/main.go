// Command pulse serves Project Pulse: live GitHub analytics for your projects.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KING-CYBERTON/project-pulse/internal/config"
	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
	"github.com/KING-CYBERTON/project-pulse/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("pulse: ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Token == "" {
		log.Print("warning: GITHUB_TOKEN is not set; private repos will fail and the rate limit is 60 requests/hour")
	}
	client := gh.New(cfg.APIBase, cfg.Token)
	svc := server.NewService(cfg, client, nil)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(svc, client),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      3 * time.Minute,
	}

	// Warm the cache so the first page load is fast.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if snap, err := svc.Snapshot(ctx, false); err == nil {
			log.Printf("loaded %d projects in %s", len(snap.Reports), snap.Took.Round(time.Millisecond))
		}
	}()

	go func() {
		log.Printf("Project Pulse %s listening on %s (%d projects, %d-day lookback)", server.Version, cfg.Addr, len(cfg.Projects), cfg.LookbackDays)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Print("stopped")
}
