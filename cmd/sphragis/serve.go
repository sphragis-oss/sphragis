// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sphragis-oss/sphragis/internal/anchor"
	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/config"
	"github.com/sphragis-oss/sphragis/internal/control"
	"github.com/sphragis-oss/sphragis/internal/proxy"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Short:   "Run the gateway in the foreground",
		GroupID: groupGateway,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return serve(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		},
	}
}

func serve(logger *slog.Logger) error {
	cfg := config.FromEnv()
	if cfg.CustomTermsFile != "" {
		terms, err := config.LoadCustomTerms(cfg.CustomTermsFile)
		if err != nil {
			return err
		}
		redact.Configure(terms)
		logger.Info("loaded custom redaction terms", "count", len(terms))
	}
	if cfg.NERURL != "" {
		redact.ConfigureNER(cfg.NERURL)
		logger.Info("external NER detector enabled", "url", cfg.NERURL)
	}
	log, err := audit.Open(cfg.AuditLogPath)
	if err != nil {
		return err
	}
	defer log.Close()

	mux := http.NewServeMux()
	mux.Handle("/v1/", proxy.New(cfg.AnthropicBaseURL, cfg.OpenAIBaseURL, cfg.UpstreamBaseURL, cfg.UpstreamAPIKey, log, logger))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	stopCh := make(chan struct{})
	go autoAnchorLoop(cfg, logger, stopCh)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		close(stopCh)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	logger.Info("sphragis listening", "addr", cfg.ListenAddr, "anthropic", cfg.AnthropicBaseURL, "openai", cfg.OpenAIBaseURL, "audit_log", cfg.AuditLogPath)
	err = srv.ListenAndServe()
	if pid, ok := control.ReadPID(); ok && pid == os.Getpid() {
		control.RemovePID()
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func autoAnchorLoop(cfg config.Config, logger *slog.Logger, stop <-chan struct{}) {
	for {
		d, err := time.ParseDuration(control.LoadState().Interval)
		if err != nil {
			d = 24 * time.Hour
		}
		select {
		case <-stop:
			return
		case <-time.After(d):
		}
		if !control.LoadState().AutoAnchor {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		root, err := anchor.Anchor(ctx, cfg.AuditLogPath, cfg.AuditLogPath+".ots", cfg.OTSCalendars, nil)
		cancel()
		if err != nil {
			logger.Error("auto-anchor failed", "err", err)
		} else {
			logger.Info("auto-anchored", "root", root)
		}
	}
}
