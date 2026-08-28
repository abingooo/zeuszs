package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSocketPath          = "/run/zeuszs-updater/updater.sock"
	defaultReleaseRoot         = "/opt/zeuszs/releases"
	defaultBackupRoot          = "/opt/zeuszs/backups"
	defaultStateRoot           = "/var/lib/zeuszs-updater"
	defaultComposeDir          = "/opt/new-api"
	defaultComposeFile         = "/opt/new-api/docker-compose.yml"
	defaultComposeOverrideFile = "/opt/new-api/docker-compose.override.yml"
	defaultDockerBinary        = "/usr/bin/docker"
	defaultDockerRoot          = "/var/lib/docker"
	defaultHealthURL           = "http://127.0.0.1:3000/api/status"
	defaultBaseImage           = "debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a"
	defaultMinFreeBytes        = 2 << 30
	minimumTokenBytes          = 32
)

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type config struct {
	SocketPath          string
	Token               string
	ReleaseRoot         string
	BackupRoot          string
	StateRoot           string
	ComposeDir          string
	ComposeFile         string
	ComposeOverrideFile string
	DockerBinary        string
	DockerRoot          string
	AppService          string
	PostgresContainer   string
	PostgresUser        string
	PostgresDatabase    string
	HealthURL           string
	BaseImage           string
	GOOS                string
	GOARCH              string
	HealthTimeout       time.Duration
	PollInterval        time.Duration
	UpdateTimeout       time.Duration
	RollbackTimeout     time.Duration
	MinFreeBytes        uint64
}

func loadConfigFromEnv() (config, error) {
	cfg := config{
		SocketPath:          envOrDefault("ZEUSZS_UPDATER_SOCKET", defaultSocketPath),
		Token:               strings.TrimSpace(os.Getenv("ZEUSZS_UPDATER_TOKEN")),
		ReleaseRoot:         envOrDefault("ZEUSZS_UPDATER_RELEASE_ROOT", defaultReleaseRoot),
		BackupRoot:          envOrDefault("ZEUSZS_UPDATER_BACKUP_ROOT", defaultBackupRoot),
		StateRoot:           envOrDefault("ZEUSZS_UPDATER_STATE_ROOT", defaultStateRoot),
		ComposeDir:          envOrDefault("ZEUSZS_UPDATER_COMPOSE_DIR", defaultComposeDir),
		ComposeFile:         envOrDefault("ZEUSZS_UPDATER_COMPOSE_FILE", defaultComposeFile),
		ComposeOverrideFile: envOrDefault("ZEUSZS_UPDATER_COMPOSE_OVERRIDE_FILE", defaultComposeOverrideFile),
		DockerBinary:        envOrDefault("ZEUSZS_UPDATER_DOCKER_BIN", defaultDockerBinary),
		DockerRoot:          envOrDefault("ZEUSZS_UPDATER_DOCKER_ROOT", defaultDockerRoot),
		AppService:          envOrDefault("ZEUSZS_UPDATER_APP_SERVICE", "new-api"),
		PostgresContainer:   envOrDefault("ZEUSZS_UPDATER_POSTGRES_CONTAINER", "new-api-postgres"),
		PostgresUser:        envOrDefault("ZEUSZS_UPDATER_POSTGRES_USER", "root"),
		PostgresDatabase:    envOrDefault("ZEUSZS_UPDATER_POSTGRES_DATABASE", "new-api"),
		HealthURL:           envOrDefault("ZEUSZS_UPDATER_HEALTH_URL", defaultHealthURL),
		BaseImage:           envOrDefault("ZEUSZS_UPDATER_BASE_IMAGE", defaultBaseImage),
		GOOS:                runtime.GOOS,
		GOARCH:              runtime.GOARCH,
		HealthTimeout:       5 * time.Minute,
		PollInterval:        2 * time.Second,
		UpdateTimeout:       15 * time.Minute,
		RollbackTimeout:     5 * time.Minute,
		MinFreeBytes:        defaultMinFreeBytes,
	}

	var err error
	cfg.HealthTimeout, err = envDuration("ZEUSZS_UPDATER_HEALTH_TIMEOUT", cfg.HealthTimeout)
	if err != nil {
		return config{}, err
	}
	cfg.PollInterval, err = envDuration("ZEUSZS_UPDATER_POLL_INTERVAL", cfg.PollInterval)
	if err != nil {
		return config{}, err
	}
	cfg.UpdateTimeout, err = envDuration("ZEUSZS_UPDATER_UPDATE_TIMEOUT", cfg.UpdateTimeout)
	if err != nil {
		return config{}, err
	}
	cfg.RollbackTimeout, err = envDuration("ZEUSZS_UPDATER_ROLLBACK_TIMEOUT", cfg.RollbackTimeout)
	if err != nil {
		return config{}, err
	}
	cfg.MinFreeBytes, err = envUint64("ZEUSZS_UPDATER_MIN_FREE_BYTES", cfg.MinFreeBytes)
	if err != nil {
		return config{}, err
	}
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (cfg config) validate() error {
	if len(cfg.Token) < minimumTokenBytes || cfg.Token == "replace-with-a-long-random-token" {
		return fmt.Errorf("ZEUSZS_UPDATER_TOKEN must contain at least %d non-placeholder bytes", minimumTokenBytes)
	}
	for name, path := range map[string]string{
		"socket":                cfg.SocketPath,
		"release root":          cfg.ReleaseRoot,
		"backup root":           cfg.BackupRoot,
		"state root":            cfg.StateRoot,
		"compose dir":           cfg.ComposeDir,
		"compose file":          cfg.ComposeFile,
		"compose override file": cfg.ComposeOverrideFile,
		"docker binary":         cfg.DockerBinary,
		"docker root":           cfg.DockerRoot,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%s must be an absolute path", name)
		}
	}
	for name, value := range map[string]string{
		"app service":        cfg.AppService,
		"postgres container": cfg.PostgresContainer,
		"postgres user":      cfg.PostgresUser,
		"postgres database":  cfg.PostgresDatabase,
	} {
		if !serviceNamePattern.MatchString(value) {
			return fmt.Errorf("invalid %s", name)
		}
	}
	healthURL, err := url.Parse(cfg.HealthURL)
	if err != nil || healthURL.Scheme != "http" || healthURL.Host == "" {
		return errors.New("health URL must be an absolute http URL")
	}
	if cfg.HealthTimeout <= 0 || cfg.PollInterval <= 0 || cfg.PollInterval > cfg.HealthTimeout {
		return errors.New("health timeout and poll interval are invalid")
	}
	if cfg.UpdateTimeout <= 0 || cfg.RollbackTimeout <= 0 {
		return errors.New("update and rollback timeouts must be positive")
	}
	if cfg.MinFreeBytes == 0 {
		return errors.New("minimum free bytes must be positive")
	}
	if cfg.BaseImage == "" || strings.ContainsAny(cfg.BaseImage, " \t\r\n") {
		return errors.New("base image is invalid")
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return duration, nil
}

func envUint64(name string, fallback uint64) (uint64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
