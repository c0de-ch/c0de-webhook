package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/config"
	"c0de-webhook/internal/mail"
	"c0de-webhook/internal/queue"
	"c0de-webhook/internal/store"
	"c0de-webhook/internal/ui"
	"c0de-webhook/internal/webhook"
	"c0de-webhook/web"
)

func main() {
	configPath := flag.String("config", "", "path to config file (or set WEBHOOK_CONFIG)")
	flag.Parse()

	if *configPath == "" {
		*configPath = os.Getenv("WEBHOOK_CONFIG")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure database directory exists
	if err := os.MkdirAll("data", 0755); err != nil && !os.IsExist(err) {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize store
	st, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer st.Close()

	// Initialize auth
	a := auth.New(st, cfg.Server.AdminPassword, cfg.Server.SecretKey)

	// Initialize SMTP sender
	sender := mail.NewSMTPSender(cfg.SMTP)

	// Initialize queue workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := queue.NewWorker(st, sender, cfg.Queue.Workers, cfg.Queue.RetryDuration)
	w.Start(ctx)

	// Session cleanup ticker
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.CleanupSessions()
			}
		}
	}()

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Web UI
	webFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		log.Fatalf("Failed to setup web filesystem: %v", err)
	}
	uiHandler := ui.NewHandler(st, a, cfg, webFS)
	uiHandler.RegisterRoutes(mux)

	// API
	webhookHandler := webhook.NewHandler(st, a, cfg.Queue.MaxRetries)
	webhookHandler.RegisterRoutes(mux)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		w.Stop()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("Starting c0de-webhook on %s", addr)
	log.Printf("Admin UI: http://%s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
