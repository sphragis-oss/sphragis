package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/sphragis-oss/sphragis/internal/anchor"
	"github.com/sphragis-oss/sphragis/internal/audit"
	"github.com/sphragis-oss/sphragis/internal/config"
	"github.com/sphragis-oss/sphragis/internal/control"
	"github.com/sphragis-oss/sphragis/internal/proxy"
	"github.com/sphragis-oss/sphragis/internal/redact"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "start":
		err = start()
	case "stop":
		err = stop()
	case "restart":
		_ = stop()
		err = start()
	case "status":
		printStatus()
	case "serve":
		err = serve(logger)
	case "verify":
		err = doVerify()
	case "anchor":
		err = doAnchor()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func start() error {
	if pid, ok := control.Running(); ok {
		return fmt.Errorf("already running (pid %d)", pid)
	}
	if err := control.EnsureHome(); err != nil {
		return err
	}
	logf, err := os.OpenFile(control.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "serve")
	cmd.Env = os.Environ()
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := control.WritePID(cmd.Process.Pid); err != nil {
		return err
	}
	fmt.Printf("sphragis started (pid %d)\nlogs: %s\n", cmd.Process.Pid, control.LogPath())
	return nil
}

func stop() error {
	pid, ok := control.Running()
	if !ok {
		control.RemovePID()
		return fmt.Errorf("not running")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		if _, ok := control.Running(); !ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	control.RemovePID()
	fmt.Printf("sphragis stopped (pid %d)\n", pid)
	return nil
}

func printStatus() {
	cfg := config.FromEnv()
	st := control.LoadState()
	if pid, ok := control.Running(); ok {
		fmt.Printf("status:       running (pid %d)\n", pid)
	} else {
		fmt.Println("status:       stopped")
	}
	fmt.Printf("listen:       %s\n", cfg.ListenAddr)
	fmt.Printf("upstream:     %s\n", cfg.UpstreamBaseURL)
	fmt.Printf("audit log:    %s\n", cfg.AuditLogPath)
	fmt.Printf("auto-anchor:  %v (interval %s)\n", st.AutoAnchor, st.Interval)
	if cfg.NERURL != "" {
		fmt.Printf("NER:          %s\n", cfg.NERURL)
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
	mux.Handle("/v1/", proxy.New(cfg.UpstreamBaseURL, cfg.UpstreamAPIKey, log, logger))
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

	logger.Info("sphragis listening", "addr", cfg.ListenAddr, "upstream", cfg.UpstreamBaseURL, "audit_log", cfg.AuditLogPath)
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

func doVerify() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: sphragis verify <audit-log-path>")
	}
	n, root, err := audit.Verify(os.Args[2])
	if err != nil {
		return err
	}
	fmt.Printf("OK: %d records, chain intact\nmerkle_root: %s\n", n, root)
	return nil
}

func doAnchor() error {
	sub := ""
	if len(os.Args) >= 3 {
		sub = os.Args[2]
	}
	switch sub {
	case "on":
		st := control.LoadState()
		st.AutoAnchor = true
		if len(os.Args) >= 4 {
			st.Interval = os.Args[3]
		}
		if err := control.SaveState(st); err != nil {
			return err
		}
		fmt.Printf("auto-anchor: on (interval %s)\n", st.Interval)
		return nil
	case "off":
		st := control.LoadState()
		st.AutoAnchor = false
		if err := control.SaveState(st); err != nil {
			return err
		}
		fmt.Println("auto-anchor: off")
		return nil
	case "status":
		st := control.LoadState()
		fmt.Printf("auto-anchor: %v (interval %s)\n", st.AutoAnchor, st.Interval)
		return nil
	}
	cfg := config.FromEnv()
	logPath := cfg.AuditLogPath
	if sub == "now" {
		if len(os.Args) >= 4 {
			logPath = os.Args[3]
		}
	} else if sub != "" {
		logPath = sub
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := anchor.Anchor(ctx, logPath, logPath+".ots", cfg.OTSCalendars, nil)
	if err != nil {
		return err
	}
	fmt.Printf("anchored merkle_root %s\nproof: %s (pending; run `ots upgrade %s` later)\n", root, logPath+".ots", logPath+".ots")
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `sphragis - EU AI Act compliance gateway

usage:
  sphragis start                 start the gateway in the background
  sphragis stop                  stop the background gateway
  sphragis restart               restart the background gateway
  sphragis status                show daemon and config status
  sphragis serve                 run in the foreground
  sphragis verify <log>          verify the audit log chain
  sphragis anchor on [interval]  enable automatic anchoring (e.g. 24h)
  sphragis anchor off            disable automatic anchoring
  sphragis anchor status         show auto-anchor state
  sphragis anchor now [log]      anchor once now`)
}
