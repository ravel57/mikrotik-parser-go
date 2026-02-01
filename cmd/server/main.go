package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/cors"

	"mikrotik-parser-go/internal/config"
	httpapi "mikrotik-parser-go/internal/http"
	imigrate "mikrotik-parser-go/internal/migrate"
	"mikrotik-parser-go/internal/mikrotik"
	"mikrotik-parser-go/internal/service"
	"mikrotik-parser-go/internal/storage"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := imigrate.Up(cfg.SqliteDSN); err != nil {
		log.Fatal(err)
	}

	pg, err := storage.New(ctx, cfg.SqliteDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Close()

	mt := mikrotik.New(cfg.MikrotikAddr, cfg.MikrotikUser, cfg.MikrotikPass)
	defer mt.Close()

	connectionsSvc := service.NewConnectionsService(mt, cfg.IgnoreVPNListName, cfg.IgnoreLanToVpnListName)
	collectSvc := service.NewCollectService(connectionsSvc, pg, cfg.CollectInterval)

	go collectSvc.Run(ctx)

	h := httpapi.NewHandler(connectionsSvc, collectSvc, cfg.StaticDir)
	handler := cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
		MaxAge:           300,
	})(h.Router())

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("HTTP listening on", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	cancel()
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(ctxShutdown)
}
