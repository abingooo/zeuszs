package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func (updater *releaseUpdater) waitForHealthy(ctx context.Context, expectedVersion string) error {
	deadline := time.Now().Add(updater.cfg.HealthTimeout)
	var lastDetail string
	for {
		if time.Now().After(deadline) {
			if lastDetail == "" {
				lastDetail = "no health result"
			}
			return fmt.Errorf("app did not become healthy with version %s: %s", expectedVersion, lastDetail)
		}
		containerID, err := updater.runDocker(ctx, "", updater.composeArgs("ps", "-q", updater.cfg.AppService)...)
		if err != nil {
			lastDetail = err.Error()
		} else if strings.TrimSpace(containerID) == "" {
			lastDetail = "app container is not running"
		} else {
			health, inspectErr := updater.runDocker(ctx, "", "inspect", "--format={{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", strings.TrimSpace(containerID))
			if inspectErr != nil {
				lastDetail = inspectErr.Error()
			} else if strings.TrimSpace(health) != "healthy" {
				lastDetail = "container state is " + strings.TrimSpace(health)
			} else if expectedVersion == "" {
				return nil
			} else {
				version, versionErr := updater.health.Version(ctx, updater.cfg.HealthURL)
				if versionErr != nil {
					lastDetail = versionErr.Error()
				} else if version != expectedVersion {
					return fmt.Errorf("/api/status reported version %q, expected %q", version, expectedVersion)
				} else {
					return nil
				}
			}
		}

		timer := time.NewTimer(updater.cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type apiHealthClient struct {
	client *http.Client
}

func (client apiHealthClient) Version(ctx context.Context, endpoint string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create status request: %w", err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request /api/status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("/api/status returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := common.DecodeJson(io.LimitReader(response.Body, 1024*1024), &payload); err != nil {
		return "", fmt.Errorf("decode /api/status response: %w", err)
	}
	if !payload.Success || payload.Data.Version == "" {
		return "", errors.New("/api/status did not return a successful version")
	}
	return payload.Data.Version, nil
}
