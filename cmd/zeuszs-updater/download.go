package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
)

const (
	githubReleaseDownloadRoot = "https://github.com/abingooo/zeuszs/releases/download"
	maxChecksumAssetBytes     = 1 << 20
	maxBinaryAssetBytes       = 512 << 20
	maxLicenseAssetBytes      = 8 << 20
)

var releaseTagPattern = regexp.MustCompile(`^zeuszs-v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$`)

type downloader interface {
	Download(context.Context, string, string, int64) error
}

type httpDownloader struct {
	client *http.Client
}

func (downloader httpDownloader) Download(ctx context.Context, sourceURL, destination string, maxBytes int64) error {
	if maxBytes <= 0 {
		return errors.New("release asset size limit must be positive")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create release download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "zeuszs-updater")
	response, err := downloader.client.Do(request)
	if err != nil {
		return fmt.Errorf("download release asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("download release asset: unexpected HTTP status %d", response.StatusCode)
	}

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create release asset: %w", err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("write release asset: %w", copyErr)
	}
	if written > maxBytes {
		_ = os.Remove(destination)
		return fmt.Errorf("release asset exceeds the %d-byte size limit", maxBytes)
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("close release asset: %w", closeErr)
	}
	return nil
}

func validReleaseTag(tag string) bool {
	_, err := parseReleaseVersion(tag)
	return err == nil
}

func parseReleaseVersion(tag string) (*goversion.Version, error) {
	if len(tag) > 128 || !releaseTagPattern.MatchString(tag) {
		return nil, errors.New("release tag must match zeuszs-v<major>.<minor>.<patch>")
	}
	return goversion.NewSemver(strings.TrimPrefix(tag, "zeuszs-v"))

}

func parseInstalledVersion(raw string) (*goversion.Version, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "zeuszs-")
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return nil, errors.New("installed version is empty")
	}
	return goversion.NewSemver(value)
}

func releaseAssetName(goos, goarch, tag string) (string, error) {
	if goos != "linux" {
		return "", fmt.Errorf("unsupported operating system %q", goos)
	}
	switch goarch {
	case "amd64":
		return "new-api-" + tag, nil
	case "arm64":
		return "new-api-arm64-" + tag, nil
	default:
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
}

func imageTagForRelease(tag string) string {
	return strings.ReplaceAll(strings.TrimPrefix(tag, "zeuszs-"), "+", "_")
}

func (updater *releaseUpdater) downloadRelease(ctx context.Context, tag string) (string, string, error) {
	assetName, err := releaseAssetName(updater.cfg.GOOS, updater.cfg.GOARCH, tag)
	if err != nil {
		return "", "", err
	}
	releaseDir := filepath.Join(updater.cfg.ReleaseRoot, tag)
	if err := ensureDirectory(releaseDir, 0o700); err != nil {
		return "", "", err
	}

	checksumPath := filepath.Join(releaseDir, "checksums-linux.txt")
	binaryPath := filepath.Join(releaseDir, assetName)
	if err := updater.downloadAsset(ctx, tag, "checksums-linux.txt", checksumPath); err != nil {
		return "", "", err
	}
	if err := updater.downloadAsset(ctx, tag, assetName, binaryPath); err != nil {
		return "", "", err
	}
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return "", "", fmt.Errorf("read release checksums: %w", err)
	}
	want, err := checksumForAsset(checksumData, assetName)
	if err != nil {
		return "", "", err
	}
	if err := verifyFileSHA256(binaryPath, want); err != nil {
		return "", "", err
	}
	if err := os.Chmod(binaryPath, 0o555); err != nil {
		return "", "", fmt.Errorf("make release binary executable: %w", err)
	}
	for _, licenseName := range []string{"LICENSE", "NOTICE", "THIRD-PARTY-LICENSES.md"} {
		licensePath := filepath.Join(releaseDir, licenseName)
		if err := updater.downloadAsset(ctx, tag, licenseName, licensePath); err != nil {
			return "", "", err
		}
		wantLicense, err := checksumForAsset(checksumData, licenseName)
		if err != nil {
			return "", "", err
		}
		if err := verifyFileSHA256(licensePath, wantLicense); err != nil {
			return "", "", err
		}
		if err := os.Chmod(licensePath, 0o444); err != nil {
			return "", "", fmt.Errorf("set release license permissions: %w", err)
		}
	}
	return releaseDir, binaryPath, nil
}

func (updater *releaseUpdater) downloadAsset(ctx context.Context, tag, assetName, destination string) error {
	temporary := destination + ".download"
	_ = os.Remove(temporary)
	sourceURL := githubReleaseDownloadRoot + "/" + tag + "/" + assetName
	maxBytes, err := releaseAssetSizeLimit(assetName)
	if err != nil {
		return err
	}
	if err := updater.downloader.Download(ctx, sourceURL, temporary, maxBytes); err != nil {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("install release asset: %w", err)
	}
	return nil
}

func releaseAssetSizeLimit(assetName string) (int64, error) {
	switch assetName {
	case "checksums-linux.txt":
		return maxChecksumAssetBytes, nil
	case "LICENSE", "NOTICE", "THIRD-PARTY-LICENSES.md":
		return maxLicenseAssetBytes, nil
	default:
		if strings.HasPrefix(assetName, "new-api-") {
			return maxBinaryAssetBytes, nil
		}
		return 0, fmt.Errorf("unsupported release asset %q", assetName)
	}
}

func checksumForAsset(checksumData []byte, assetName string) ([]byte, error) {
	lines := strings.Split(string(checksumData), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		if len(fields[0]) != sha256.Size*2 {
			return nil, errors.New("release checksum has an invalid SHA256 value")
		}
		checksum, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, errors.New("release checksum has an invalid SHA256 value")
		}
		return checksum, nil
	}
	return nil, fmt.Errorf("release checksum does not contain %s", assetName)
}

func verifyFileSHA256(path string, want []byte) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open release binary for verification: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash release binary: %w", err)
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), want) != 1 {
		return errors.New("release binary SHA256 does not match checksums-linux.txt")
	}
	return nil
}

func ensureDirectory(path string, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path is not a directory: %s", path)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect directory %s: %w", path, err)
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}

func newHTTPDownloader() httpDownloader {
	return httpDownloader{client: &http.Client{Timeout: 10 * time.Minute}}
}
