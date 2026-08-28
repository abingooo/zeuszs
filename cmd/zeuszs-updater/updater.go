package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type healthClient interface {
	Version(context.Context, string) (string, error)
}

type releaseUpdater struct {
	cfg        config
	runner     commandRunner
	downloader downloader
	health     healthClient
	now        func() time.Time
}

func (updater *releaseUpdater) Execute(tag string, reportStep func(string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), updater.cfg.UpdateTimeout)
	defer cancel()
	targetVersion, err := parseReleaseVersion(tag)
	if err != nil {
		return err
	}

	reportStep("resolve-current-version")
	currentVersionText, err := updater.health.Version(ctx, updater.cfg.HealthURL)
	if err != nil {
		return fmt.Errorf("resolve current app version: %w", err)
	}
	currentVersion, err := parseInstalledVersion(currentVersionText)
	if err != nil {
		return fmt.Errorf("parse current app version %q: %w", currentVersionText, err)
	}
	if !targetVersion.GreaterThan(currentVersion) {
		return fmt.Errorf("target version %s must be newer than current version %s", tag, currentVersionText)
	}

	reportStep("validate-compose")
	if err := updater.validateComposeTarget(ctx); err != nil {
		return err
	}

	reportStep("check-disk-space")
	if err := updater.ensureFreeDisk(); err != nil {
		return err
	}

	reportStep("resolve-current-image")
	oldImageID, err := updater.runDocker(ctx, "", "image", "inspect", "--format={{.Id}}", "zeuszs:stable")
	if err != nil {
		return fmt.Errorf("resolve current stable image: %w", err)
	}
	oldImageID = strings.TrimSpace(oldImageID)
	if oldImageID == "" {
		return errors.New("current zeuszs:stable image has no image ID")
	}

	reportStep("download-release")
	releaseDir, binaryPath, err := updater.downloadRelease(ctx, tag)
	if err != nil {
		return err
	}

	reportStep("backup-compose")
	backupDir, err := updater.createBackup(tag)
	if err != nil {
		return err
	}

	reportStep("backup-postgres")
	if err := updater.backupPostgres(ctx, backupDir); err != nil {
		return err
	}

	reportStep("build-image")
	versionImage := "zeuszs:" + imageTagForRelease(tag)
	if err := updater.writeDockerfile(releaseDir, filepath.Base(binaryPath)); err != nil {
		return err
	}
	if _, err := updater.runDocker(ctx, "", "build", "--tag", versionImage, "--file", filepath.Join(releaseDir, "Dockerfile"), releaseDir); err != nil {
		return fmt.Errorf("build release image: %w", err)
	}

	reportStep("verify-image")
	output, err := updater.runDocker(
		ctx,
		"",
		"run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--entrypoint", "/usr/local/bin/new-api",
		versionImage, "--version",
	)
	if err != nil {
		return fmt.Errorf("run release image version check: %w", err)
	}
	if strings.TrimSpace(output) != tag {
		return fmt.Errorf("release image reported version %q, expected %q", strings.TrimSpace(output), tag)
	}

	stableChanged := false
	rollback := func(updateErr error) error {
		if !stableChanged {
			return updateErr
		}
		reportStep("rollback")
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), updater.cfg.RollbackTimeout)
		defer rollbackCancel()
		if _, err := updater.runDocker(rollbackCtx, "", "tag", oldImageID, "zeuszs:stable"); err != nil {
			return fmt.Errorf("%w; rollback stable tag failed: %v", updateErr, err)
		}
		if _, err := updater.composeUp(rollbackCtx); err != nil {
			return fmt.Errorf("%w; rollback app recreation failed: %v", updateErr, err)
		}
		if err := updater.waitForHealthy(rollbackCtx, ""); err != nil {
			return fmt.Errorf("%w; rollback app health check failed: %v", updateErr, err)
		}
		return updateErr
	}

	reportStep("activate-image")
	stableChanged = true
	if _, err := updater.runDocker(ctx, "", "tag", versionImage, "zeuszs:stable"); err != nil {
		return rollback(fmt.Errorf("tag release image as stable: %w", err))
	}

	reportStep("recreate-app")
	if _, err := updater.composeUp(ctx); err != nil {
		return rollback(fmt.Errorf("recreate app service: %w", err))
	}

	reportStep("verify-health")
	if err := updater.waitForHealthy(ctx, tag); err != nil {
		return rollback(err)
	}
	return nil
}

func (updater *releaseUpdater) runDocker(ctx context.Context, dir string, args ...string) (string, error) {
	return updater.runner.Run(ctx, command{Name: updater.cfg.DockerBinary, Args: args, Dir: dir})
}

func (updater *releaseUpdater) ensureFreeDisk() error {
	if updater.cfg.MinFreeBytes == 0 {
		return errors.New("minimum free disk space is not configured")
	}
	seen := make(map[string]struct{}, 3)
	for _, path := range []string{updater.cfg.ReleaseRoot, updater.cfg.BackupRoot, updater.cfg.DockerRoot} {
		cleanPath := filepath.Clean(path)
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}
		if err := ensureDirectory(cleanPath, 0o700); err != nil {
			return err
		}
		available, err := availableDiskBytes(cleanPath)
		if err != nil {
			return err
		}
		if available < updater.cfg.MinFreeBytes {
			return fmt.Errorf("insufficient free disk space at %s: have %d bytes, require at least %d", cleanPath, available, updater.cfg.MinFreeBytes)
		}
	}
	return nil
}
