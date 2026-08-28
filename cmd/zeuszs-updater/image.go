package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func (updater *releaseUpdater) writeDockerfile(releaseDir, binaryName string) error {
	content := fmt.Sprintf(`FROM %s
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates
COPY %s /usr/local/bin/new-api
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/new-api"]
`, updater.cfg.BaseImage, binaryName)
	path := filepath.Join(releaseDir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write release Dockerfile: %w", err)
	}
	return nil
}

func (updater *releaseUpdater) composeUp(ctx context.Context) (string, error) {
	return updater.runDocker(ctx, "", updater.composeArgs("up", "-d", "--no-deps", "--force-recreate", updater.cfg.AppService)...)
}

func (updater *releaseUpdater) composeArgs(args ...string) []string {
	base := []string{
		"compose",
		"--file", updater.cfg.ComposeFile,
		"--file", updater.cfg.ComposeOverrideFile,
		"--project-directory", updater.cfg.ComposeDir,
	}
	return append(base, args...)
}

func (updater *releaseUpdater) validateComposeTarget(ctx context.Context) error {
	output, err := updater.runDocker(ctx, "", updater.composeArgs("config", "--format", "json")...)
	if err != nil {
		return fmt.Errorf("resolve Compose deployment: %w", err)
	}
	var project struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := common.Unmarshal([]byte(output), &project); err != nil {
		return fmt.Errorf("decode Compose deployment: %w", err)
	}
	service, ok := project.Services[updater.cfg.AppService]
	if !ok {
		return fmt.Errorf("Compose service %q does not exist", updater.cfg.AppService)
	}
	if strings.TrimSpace(service.Image) == "" {
		return errors.New("Compose app service does not declare an image")
	}
	if service.Image != "zeuszs:stable" {
		return fmt.Errorf("Compose service %q must use exactly zeuszs:stable, got %q", updater.cfg.AppService, service.Image)
	}
	return nil
}
