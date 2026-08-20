// Command task138-railblock serves the railway-interlocking HTTP API backed by
// SQLite, and provides a --smoke-test that exercises the full contract (station
// + yard elements, route definition & conflict graph, route establishment with
// switch locking & signal open, train approach-lock, three-point sectional
// release, timed manual cancel, fault unlock, restart recovery, frontend page)
// without real-time sleeps.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/httpapi"
	"task138-railblock/internal/selfcheck"
	"task138-railblock/internal/service"
	"task138-railblock/internal/store"
	"task138-railblock/internal/webfs"
)

// webFS is the embedded static frontend (native HTML/CSS/JS, no build step).
// The embed lives in package webfs so the selfcheck can serve the same page.
var webFS = webfs.FS()

// DefaultAdminToken protects admin endpoints. Override with RAILBLOCK_ADMIN_TOKEN.
const DefaultAdminToken = "railblock-secret"

func main() {
	smoke := flag.Bool("smoke-test", false, "run self-check and exit")
	migrateOnly := flag.Bool("migrate-only", false, "apply schema and exit")
	dbPath := flag.String("db", "railblock.db", "SQLite database file path")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if *smoke {
		if err := selfcheck.Run(); err != nil {
			fmt.Println("smoke-test: FAIL:", err)
			osExit(1)
		}
		fmt.Println("smoke-test: ok")
		osExit(0)
	}

	adminToken := os.Getenv("RAILBLOCK_ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = DefaultAdminToken
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.NewWithClock(st, clock.Real{})
	// Restart-recovery: recompute every derived figure from the persisted
	// event history so a process that died between writes converges to the
	// same state.
	if n, err := svc.Reconcile().ReconcileAll(context.Background()); err != nil {
		log.Printf("reconcile on startup: %v", err)
	} else if n > 0 {
		log.Printf("reconciled %d derived rows on startup", n)
	}

	if *migrateOnly {
		fmt.Println("migrate-only: schema applied")
		return
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewMux(httpapi.Services{Svc: svc}, adminToken, webFS),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("railblock %s listening on %s (db=%s)", httpapi.Version, *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// osExit is indirected so tests can substitute it; in production it is os.Exit.
var osExit = os.Exit
