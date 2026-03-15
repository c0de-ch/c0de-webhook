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
	"strconv"
	"syscall"
	"time"

	"c0de-webhook/internal/auth"
	"c0de-webhook/internal/config"
	"c0de-webhook/internal/mail"
	"c0de-webhook/internal/queue"
	"c0de-webhook/internal/store"
	"c0de-webhook/internal/telegram"
	"c0de-webhook/internal/ui"
	"c0de-webhook/internal/webhook"
	"c0de-webhook/internal/whatsapp"
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

	if err := os.MkdirAll("data", 0755); err != nil && !os.IsExist(err) {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	st, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer st.Close()

	a := auth.New(st, cfg.Server.AdminPassword, cfg.Server.SecretKey)

	// Initialize WhatsApp Web client (self-hosted, free)
	var waWebClient *whatsapp.WebClient
	waWebClient, err = whatsapp.NewWebClient("./data/whatsapp.db")
	if err != nil {
		log.Printf("WARNING: WhatsApp Web init failed (CGO required): %v", err)
	}

	// Build channel senders — load config from DB settings with config file fallback
	senders := buildSenders(st, cfg)
	if waWebClient != nil {
		senders.WhatsAppWeb = waWebClient
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := queue.NewWorker(st, senders, cfg.Queue.Workers, cfg.Queue.RetryDuration)
	w.Start(ctx)

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

	mux := http.NewServeMux()

	webFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		log.Fatalf("Failed to setup web filesystem: %v", err)
	}
	reloadSenders := func() {
		s := buildSenders(st, cfg)
		if waWebClient != nil {
			s.WhatsAppWeb = waWebClient
		}
		w.UpdateSenders(s)
	}
	uiOpts := []ui.HandlerOption{ui.WithOnSettingsSaved(reloadSenders)}
	if waWebClient != nil {
		uiOpts = append(uiOpts, ui.WithWhatsAppWeb(waWebClient))
	}
	uiHandler := ui.NewHandler(st, a, cfg, webFS, uiOpts...)
	uiHandler.RegisterRoutes(mux)

	webhookHandler := webhook.NewHandler(st, a, cfg.Queue.MaxRetries)
	webhookHandler.RegisterRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		w.Stop()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Starting c0de-webhook on %s", addr)
	log.Printf("Admin UI: http://%s", addr)
	log.Printf("Channels: mail=yes whatsapp=%v telegram=%v",
		senders.WhatsApp != nil && senders.WhatsApp.Configured(),
		senders.Telegram != nil && senders.Telegram.Configured())
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func buildSenders(st *store.Store, cfg *config.Config) queue.ChannelSender {
	settings, _ := st.GetAllSettings()

	// Mail sender — DB settings override config file
	smtpCfg := cfg.SMTP
	if v, ok := settings["smtp_host"]; ok && v != "" {
		smtpCfg.Host = v
	}
	if v, ok := settings["smtp_port"]; ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			smtpCfg.Port = p
		}
	}
	if v, ok := settings["smtp_username"]; ok && v != "" {
		smtpCfg.Username = v
	}
	if v, ok := settings["smtp_password"]; ok && v != "" {
		smtpCfg.Password = v
	}
	if v, ok := settings["smtp_from"]; ok && v != "" {
		smtpCfg.From = v
	}
	if v, ok := settings["smtp_tls"]; ok {
		smtpCfg.TLS = v == "true"
	}
	if v, ok := settings["smtp_tls_skip_verify"]; ok {
		smtpCfg.TLSSkipVerify = v == "true"
	}
	if v, ok := settings["smtp_auth_method"]; ok && v != "" {
		smtpCfg.AuthMethod = v
	}

	// WhatsApp Business API
	var waSender *whatsapp.BusinessSender
	waPhoneID := settings["whatsapp_phone_id"]
	waToken := settings["whatsapp_access_token"]
	waVersion := settings["whatsapp_api_version"]
	if waPhoneID != "" && waToken != "" {
		waSender = whatsapp.NewBusinessSender(waPhoneID, waToken, waVersion)
		log.Printf("WhatsApp Business API configured (phone_id=%s)", waPhoneID)
	}

	// Telegram Bot API
	var tgSender *telegram.Sender
	tgToken := settings["telegram_bot_token"]
	tgParseMode := settings["telegram_parse_mode"]
	if tgToken != "" {
		tgSender = telegram.NewSender(tgToken, tgParseMode)
		log.Printf("Telegram Bot API configured")
	}

	return queue.ChannelSender{
		Mail:     mail.NewSMTPSender(smtpCfg),
		WhatsApp: waSender,
		Telegram: tgSender,
	}
}
