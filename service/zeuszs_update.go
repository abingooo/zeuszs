package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	version "github.com/hashicorp/go-version"
)

const (
	zeusZSRepository               = "abingooo/zeuszs"
	zeusZSReleaseTagPrefix         = "zeuszs-v"
	zeusZSUnixTriggerEndpoint      = "http://unix/v1/update"
	zeusZSUnixStatusEndpoint       = "http://unix/v1/status"
	zeusZSMaxReleaseBodyBytes      = 2 << 20
	zeusZSMaxUpdaterBodyBytes      = 64 << 10
	zeusZSUpdaterStatusTimeout     = 2 * time.Second
	zeusZSMinimumUpdaterTokenBytes = 32
)

var (
	ErrZeusZSReleaseCheckFailed    = errors.New("failed to check ZeusZS releases")
	ErrZeusZSReleaseNotFound       = errors.New("no ZeusZS release was found")
	ErrZeusZSCurrentVersionInvalid = errors.New("current ZeusZS version is invalid")
	ErrZeusZSUpdaterNotConfigured  = errors.New("ZeusZS updater is not configured")
	ErrZeusZSUpdateInProgress      = errors.New("a ZeusZS update trigger is already in progress")
	ErrZeusZSNoUpdateAvailable     = errors.New("ZeusZS is already up to date")
	ErrZeusZSUpdateTriggerFailed   = errors.New("ZeusZS update trigger failed")
	zeusZSReleaseAPIURL            = "https://api.github.com/repos/" + zeusZSRepository + "/releases?per_page=100"
	zeusZSReleaseHTTPClient        = &http.Client{Timeout: 10 * time.Second}
	zeusZSUpdateHTTPClient         = &http.Client{Timeout: 15 * time.Second, CheckRedirect: rejectZeusZSUpdateRedirect}
	zeusZSUpdateTriggerMu          sync.Mutex
	zeusZSReleaseCacheMu           sync.Mutex
	zeusZSReleaseCacheTTL          = 5 * time.Minute
	zeusZSReleaseCache             zeusZSReleaseCacheEntry
	zeusZSReleaseTagPattern        = regexp.MustCompile(`^zeuszs-v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$`)
	zeusZSNow                      = time.Now
	zeusZSCurrentVersion           = func() string { return common.Version }
	zeusZSUpdaterConfigLoader      = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{
			TriggerURL:    strings.TrimSpace(os.Getenv("ZEUSZS_UPDATE_TRIGGER_URL")),
			TriggerToken:  strings.TrimSpace(os.Getenv("ZEUSZS_UPDATE_TRIGGER_TOKEN")),
			TriggerSocket: strings.TrimSpace(os.Getenv("ZEUSZS_UPDATE_TRIGGER_SOCKET")),
		}
	}
)

type zeusZSUpdaterConfig struct {
	TriggerURL    string
	TriggerToken  string
	TriggerSocket string
}

type zeusZSReleaseCacheEntry struct {
	release   githubZeusZSRelease
	semver    *version.Version
	expiresAt time.Time
}

type githubZeusZSRelease struct {
	ID          int64                      `json:"id"`
	TagName     string                     `json:"tag_name"`
	Name        string                     `json:"name"`
	HTMLURL     string                     `json:"html_url"`
	Body        string                     `json:"body"`
	PublishedAt string                     `json:"published_at"`
	Draft       bool                       `json:"draft"`
	Prerelease  bool                       `json:"prerelease"`
	Assets      []githubZeusZSReleaseAsset `json:"assets"`
}

type githubZeusZSReleaseAsset struct {
	Name string `json:"name"`
}

type ZeusZSRelease struct {
	ID          int64  `json:"id"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}

type ZeusZSUpdateCheck struct {
	Repository        string               `json:"repository"`
	CurrentVersion    string               `json:"current_version"`
	LatestRelease     ZeusZSRelease        `json:"latest_release"`
	UpdateAvailable   bool                 `json:"update_available"`
	UpdaterConfigured bool                 `json:"updater_configured"`
	UpdaterReachable  bool                 `json:"updater_reachable"`
	UpdaterStatus     *ZeusZSUpdaterStatus `json:"updater_status"`
}

type ZeusZSUpdaterStatus struct {
	Status     string     `json:"status"`
	Tag        string     `json:"tag,omitempty"`
	Step       string     `json:"step,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type ZeusZSUpdateTriggerResult struct {
	ZeusZSUpdateCheck
	TriggeredAt string `json:"triggered_at"`
}

type zeusZSUpdateTriggerPayload struct {
	Tag string `json:"tag"`
}

func rejectZeusZSUpdateRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func CheckZeusZSUpdate(ctx context.Context) (ZeusZSUpdateCheck, error) {
	config := zeusZSUpdaterConfigLoader()
	_, _, _, configErr := zeusZSUpdaterEndpoints(config)
	result, err := checkZeusZSUpdate(ctx, zeusZSCurrentVersion(), configErr == nil)
	if err != nil {
		return ZeusZSUpdateCheck{}, err
	}
	if configErr == nil {
		result.UpdaterStatus, result.UpdaterReachable = getZeusZSUpdaterStatus(ctx, config)
	}
	return result, nil
}

func checkZeusZSUpdate(ctx context.Context, currentVersion string, updaterConfigured bool) (ZeusZSUpdateCheck, error) {
	currentSemver, err := parseZeusZSSemver(currentVersion)
	if err != nil {
		return ZeusZSUpdateCheck{}, fmt.Errorf("%w: %v", ErrZeusZSCurrentVersionInvalid, err)
	}
	latest, latestSemver, err := latestZeusZSRelease(ctx)
	if err != nil {
		return ZeusZSUpdateCheck{}, err
	}

	return ZeusZSUpdateCheck{
		Repository:     zeusZSRepository,
		CurrentVersion: currentVersion,
		LatestRelease: ZeusZSRelease{
			ID:          latest.ID,
			TagName:     latest.TagName,
			Name:        latest.Name,
			HTMLURL:     latest.HTMLURL,
			Body:        latest.Body,
			PublishedAt: latest.PublishedAt,
			Prerelease:  latest.Prerelease,
		},
		UpdateAvailable:   latestSemver.GreaterThan(currentSemver),
		UpdaterConfigured: updaterConfigured,
	}, nil
}

func latestZeusZSRelease(ctx context.Context) (githubZeusZSRelease, *version.Version, error) {
	zeusZSReleaseCacheMu.Lock()
	defer zeusZSReleaseCacheMu.Unlock()

	now := zeusZSNow()
	if zeusZSReleaseCache.semver != nil && zeusZSReleaseCache.expiresAt.After(now) {
		return zeusZSReleaseCache.release, zeusZSReleaseCache.semver, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zeusZSReleaseAPIURL, nil)
	if err != nil {
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: %v", ErrZeusZSReleaseCheckFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ZeusZS-update-checker")

	resp, err := zeusZSReleaseHTTPClient.Do(req)
	if err != nil {
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: %v", ErrZeusZSReleaseCheckFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: GitHub returned HTTP %d", ErrZeusZSReleaseCheckFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, zeusZSMaxReleaseBodyBytes+1))
	if err != nil {
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: %v", ErrZeusZSReleaseCheckFailed, err)
	}
	if len(body) > zeusZSMaxReleaseBodyBytes {
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: GitHub response is too large", ErrZeusZSReleaseCheckFailed)
	}
	var releases []githubZeusZSRelease
	if err := common.Unmarshal(body, &releases); err != nil {
		return githubZeusZSRelease{}, nil, fmt.Errorf("%w: invalid GitHub response: %v", ErrZeusZSReleaseCheckFailed, err)
	}

	var latest githubZeusZSRelease
	var latestSemver *version.Version
	for _, release := range releases {
		if release.Draft || release.Prerelease ||
			!zeusZSReleaseTagPattern.MatchString(release.TagName) ||
			!hasRequiredZeusZSReleaseAssets(release) {
			continue
		}
		candidate, parseErr := parseZeusZSSemver(release.TagName)
		if parseErr != nil || candidate.Prerelease() != "" {
			continue
		}
		if latestSemver == nil || candidate.GreaterThan(latestSemver) {
			latest = release
			latestSemver = candidate
		}
	}
	if latestSemver == nil {
		return githubZeusZSRelease{}, nil, ErrZeusZSReleaseNotFound
	}
	zeusZSReleaseCache = zeusZSReleaseCacheEntry{
		release:   latest,
		semver:    latestSemver,
		expiresAt: now.Add(zeusZSReleaseCacheTTL),
	}
	return latest, latestSemver, nil
}

func hasRequiredZeusZSReleaseAssets(release githubZeusZSRelease) bool {
	required := map[string]struct{}{
		"new-api-" + release.TagName:                    {},
		"new-api-arm64-" + release.TagName:              {},
		"zeuszs-updater-linux-amd64-" + release.TagName: {},
		"zeuszs-updater-linux-arm64-" + release.TagName: {},
		"checksums-linux.txt":                           {},
		"LICENSE":                                       {},
		"NOTICE":                                        {},
		"THIRD-PARTY-LICENSES.md":                       {},
	}
	for _, asset := range release.Assets {
		delete(required, asset.Name)
	}
	return len(required) == 0
}

func TriggerZeusZSUpdate(ctx context.Context) (ZeusZSUpdateTriggerResult, error) {
	if !zeusZSUpdateTriggerMu.TryLock() {
		return ZeusZSUpdateTriggerResult{}, ErrZeusZSUpdateInProgress
	}
	defer zeusZSUpdateTriggerMu.Unlock()

	config := zeusZSUpdaterConfigLoader()
	endpoint, _, client, err := zeusZSUpdaterEndpoints(config)
	if err != nil {
		return ZeusZSUpdateTriggerResult{}, err
	}
	check, err := checkZeusZSUpdate(ctx, zeusZSCurrentVersion(), true)
	if err != nil {
		return ZeusZSUpdateTriggerResult{}, err
	}
	if !check.UpdateAvailable {
		return ZeusZSUpdateTriggerResult{}, ErrZeusZSNoUpdateAvailable
	}

	payload, err := common.Marshal(zeusZSUpdateTriggerPayload{Tag: check.LatestRelease.TagName})
	if err != nil {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: encode request: %v", ErrZeusZSUpdateTriggerFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: create request: %v", ErrZeusZSUpdateTriggerFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+config.TriggerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ZeusZS-update-trigger")

	resp, err := client.Do(req)
	if err != nil {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: %v", ErrZeusZSUpdateTriggerFailed, err)
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, zeusZSMaxUpdaterBodyBytes+1))
	if readErr != nil {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: read response: %v", ErrZeusZSUpdateTriggerFailed, readErr)
	}
	if len(responseBody) > zeusZSMaxUpdaterBodyBytes {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: updater response is too large", ErrZeusZSUpdateTriggerFailed)
	}
	if resp.StatusCode == http.StatusConflict {
		return ZeusZSUpdateTriggerResult{}, ErrZeusZSUpdateInProgress
	}
	if resp.StatusCode != http.StatusAccepted {
		return ZeusZSUpdateTriggerResult{}, fmt.Errorf("%w: updater returned HTTP %d", ErrZeusZSUpdateTriggerFailed, resp.StatusCode)
	}
	var updaterStatus *ZeusZSUpdaterStatus
	if len(bytes.TrimSpace(responseBody)) > 0 {
		var status ZeusZSUpdaterStatus
		if err := common.Unmarshal(responseBody, &status); err == nil && status.Status != "" {
			updaterStatus = &status
		}
	}
	check.UpdaterReachable = true
	check.UpdaterStatus = updaterStatus

	return ZeusZSUpdateTriggerResult{
		ZeusZSUpdateCheck: check,
		TriggeredAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func ZeusZSUpdaterConfigured() bool {
	_, _, _, err := zeusZSUpdaterEndpoints(zeusZSUpdaterConfigLoader())
	return err == nil
}

func zeusZSUpdaterEndpoints(config zeusZSUpdaterConfig) (string, string, *http.Client, error) {
	if len(config.TriggerToken) < zeusZSMinimumUpdaterTokenBytes || config.TriggerToken == "replace-with-a-long-random-token" {
		return "", "", nil, ErrZeusZSUpdaterNotConfigured
	}
	if config.TriggerSocket != "" {
		if !filepath.IsAbs(config.TriggerSocket) {
			return "", "", nil, ErrZeusZSUpdaterNotConfigured
		}
		transport := &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				dialer := net.Dialer{Timeout: 10 * time.Second}
				return dialer.DialContext(ctx, "unix", config.TriggerSocket)
			},
		}
		return zeusZSUnixTriggerEndpoint, zeusZSUnixStatusEndpoint, &http.Client{
			Transport:     transport,
			Timeout:       15 * time.Second,
			CheckRedirect: rejectZeusZSUpdateRedirect,
		}, nil
	}
	if config.TriggerURL == "" {
		return "", "", nil, ErrZeusZSUpdaterNotConfigured
	}
	parsed, err := url.ParseRequestURI(config.TriggerURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", nil, ErrZeusZSUpdaterNotConfigured
	}
	statusURL := *parsed
	statusURL.Path = "/v1/status"
	statusURL.RawPath = ""
	statusURL.RawQuery = ""
	statusURL.Fragment = ""
	return parsed.String(), statusURL.String(), zeusZSUpdateHTTPClient, nil
}

func getZeusZSUpdaterStatus(ctx context.Context, config zeusZSUpdaterConfig) (*ZeusZSUpdaterStatus, bool) {
	_, endpoint, client, err := zeusZSUpdaterEndpoints(config)
	if err != nil {
		return nil, false
	}
	statusCtx, cancel := context.WithTimeout(ctx, zeusZSUpdaterStatusTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(statusCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+config.TriggerToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ZeusZS-update-status")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, zeusZSMaxUpdaterBodyBytes+1))
	if err != nil || len(body) > zeusZSMaxUpdaterBodyBytes {
		return nil, false
	}
	var status ZeusZSUpdaterStatus
	if err := common.Unmarshal(body, &status); err != nil || status.Status == "" {
		return nil, false
	}
	return &status, true
}

func parseZeusZSSemver(raw string) (*version.Version, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, zeusZSReleaseTagPrefix)
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return nil, errors.New("version is empty")
	}
	return version.NewSemver(value)
}
