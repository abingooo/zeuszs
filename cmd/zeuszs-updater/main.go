package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) != 1 {
		log.Fatal("usage: zeuszs-updater [--version]")
	}

	cfg, err := loadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := ensureDirectory(cfg.StateRoot, 0o700); err != nil {
		log.Fatal(err)
	}
	logFile, err := os.OpenFile(filepath.Join(cfg.StateRoot, "updater.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(os.Stdout, logFile), "", log.Ldate|log.Ltime|log.LUTC)

	listener, err := listenUnix(cfg.SocketPath)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(cfg.SocketPath)
	}()

	updater := &releaseUpdater{
		cfg:        cfg,
		runner:     execCommandRunner{},
		downloader: newHTTPDownloader(),
		health:     apiHealthClient{client: &http.Client{Timeout: 10 * time.Second}},
		now:        time.Now,
	}
	manager := newUpdateManager(updater, fileStatusStore{path: filepath.Join(cfg.StateRoot, "status.json")}, logger)
	server := &http.Server{
		Handler:           (&apiServer{token: cfg.Token, manager: manager}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("zeuszs updater %s listening on unix socket %s", version, cfg.SocketPath)
	serveErr := server.Serve(listener)
	manager.StopAndWait()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		logger.Printf("updater HTTP server failed: %v", serveErr)
	}
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to replace non-socket path %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale updater socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect updater socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on updater unix socket: %w", err)
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("set updater socket permissions: %w", err)
	}
	return listener, nil
}
