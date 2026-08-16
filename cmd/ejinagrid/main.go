// Command ejinagrid runs the Ejina Banner microgrid dispatch center HTTP service.
// It wires the domain, store, services, background worker and HTTP transport,
// seeds a small fleet of cabins if the store is empty, and listens on :51523.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/api"
	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/service"
	"github.com/ejinagrid/ejinagrid/internal/store"
	"github.com/ejinagrid/ejinagrid/internal/worker"
)

func main() {
	addr := envOrDefault("EJINA_ADDR", ":51523")
	dataPath := envOrDefault("EJINA_DATA", "./data/ejinagrid.json")
	windowStart, _ := strconv.Atoi(envOrDefault("EJINA_INSPECTION_WINDOW_START", "8"))
	windowEnd, _ := strconv.Atoi(envOrDefault("EJINA_INSPECTION_WINDOW_END", "10"))
	tickSecs, _ := strconv.Atoi(envOrDefault("EJINA_TICK_SECONDS", "30"))

	st, err := store.New(dataPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	clk := clock.System{}
	ids := &service.DefaultIDs{}
	window := domain.InspectionWindow{StartHour: windowStart, EndHour: windowEnd}
	d := service.New(st, clk, ids, window)

	seedIfEmpty(d, clk.Now())

	w := worker.New(d, st, clk, time.Duration(tickSecs)*time.Second)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go w.Run(ctx)

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(api.NewHandlers(d)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("ejinagrid listening on %s (data=%s, inspection window %02d:00-%02d:00)",
		addr, dataPath, windowStart, windowEnd)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

// seedIfEmpty populates a default three-cabin fleet when the store starts empty,
// so the service is immediately usable without external provisioning.
func seedIfEmpty(d *service.Dispatch, now time.Time) {
	if len(d.Cabins()) > 0 {
		return
	}
	defaults := []*domain.Cabin{
		domain.NewCabin("cabin-1", "1号储能舱", "ejina-site-1", 2500, now),
		domain.NewCabin("cabin-2", "2号储能舱", "ejina-site-1", 2500, now),
		domain.NewCabin("cabin-3", "3号储能舱", "ejina-site-2", 2000, now),
	}
	for _, c := range defaults {
		_ = d.SeedCabins(context.Background(), c)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
