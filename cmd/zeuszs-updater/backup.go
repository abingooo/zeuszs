package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (updater *releaseUpdater) createBackup(tag string) (string, error) {
	backupDir := filepath.Join(updater.cfg.BackupRoot, updater.now().UTC().Format("20060102T150405Z")+"-"+tag)
	if err := ensureDirectory(backupDir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("set backup directory permissions: %w", err)
	}
	files := []struct {
		path     string
		required bool
	}{
		{path: updater.cfg.ComposeFile, required: true},
		{path: updater.cfg.ComposeOverrideFile, required: true},
		{path: filepath.Join(updater.cfg.ComposeDir, ".env")},
	}
	for _, file := range files {
		if err := backupRegularFile(file.path, filepath.Join(backupDir, filepath.Base(file.path)), file.required); err != nil {
			return "", err
		}
	}
	return backupDir, nil
}

func backupRegularFile(sourcePath, destinationPath string, required bool) error {
	info, err := os.Lstat(sourcePath)
	if errors.Is(err, os.ErrNotExist) && !required {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect deployment file %s: %w", sourcePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("deployment file must be a regular non-symlink file: %s", sourcePath)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open deployment file for backup: %w", err)
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened deployment file: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return fmt.Errorf("deployment file changed while opening: %s", sourcePath)
	}
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create deployment file backup: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return fmt.Errorf("copy deployment file backup: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close deployment file backup: %w", closeErr)
	}
	return nil
}

func (updater *releaseUpdater) backupPostgres(ctx context.Context, backupDir string) error {
	path := filepath.Join(backupDir, "postgres.sql.gz")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create PostgreSQL backup: %w", err)
	}
	gzipWriter := gzip.NewWriter(file)
	_, runErr := updater.runner.Run(ctx, command{
		Name: updater.cfg.DockerBinary,
		Args: []string{
			"exec", updater.cfg.PostgresContainer,
			"pg_dump", "--clean", "--if-exists", "--no-owner", "--no-privileges",
			"--username", updater.cfg.PostgresUser,
			"--dbname", updater.cfg.PostgresDatabase,
		},
		Dir:    updater.cfg.ComposeDir,
		Stdout: gzipWriter,
	})
	gzipErr := gzipWriter.Close()
	closeErr := file.Close()
	if runErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("backup PostgreSQL: %w", runErr)
	}
	if gzipErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("compress PostgreSQL backup: %w", gzipErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close PostgreSQL backup: %w", closeErr)
	}
	return nil
}
