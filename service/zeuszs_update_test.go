package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testZeusZSUpdaterToken = "0123456789abcdef0123456789abcdef"

func setupZeusZSUpdateTest(t *testing.T) {
	t.Helper()
	previousReleaseURL := zeusZSReleaseAPIURL
	previousReleaseClient := zeusZSReleaseHTTPClient
	previousUpdateClient := zeusZSUpdateHTTPClient
	previousVersion := zeusZSCurrentVersion
	previousConfigLoader := zeusZSUpdaterConfigLoader
	previousCacheTTL := zeusZSReleaseCacheTTL
	previousNow := zeusZSNow
	zeusZSUpdateTriggerMu = sync.Mutex{}
	zeusZSReleaseCacheMu.Lock()
	zeusZSReleaseCache = zeusZSReleaseCacheEntry{}
	zeusZSReleaseCacheMu.Unlock()
	t.Cleanup(func() {
		zeusZSReleaseAPIURL = previousReleaseURL
		zeusZSReleaseHTTPClient = previousReleaseClient
		zeusZSUpdateHTTPClient = previousUpdateClient
		zeusZSCurrentVersion = previousVersion
		zeusZSUpdaterConfigLoader = previousConfigLoader
		zeusZSReleaseCacheTTL = previousCacheTTL
		zeusZSNow = previousNow
		zeusZSUpdateTriggerMu = sync.Mutex{}
		zeusZSReleaseCacheMu.Lock()
		zeusZSReleaseCache = zeusZSReleaseCacheEntry{}
		zeusZSReleaseCacheMu.Unlock()
	})
}

func zeusZSReleaseServer(t *testing.T, releases []githubZeusZSRelease) *httptest.Server {
	t.Helper()
	for i := range releases {
		if releases[i].Assets == nil {
			releases[i].Assets = testZeusZSReleaseAssets(releases[i].TagName)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
		payload, err := common.Marshal(releases)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

func testZeusZSReleaseAssets(tag string) []githubZeusZSReleaseAsset {
	return []githubZeusZSReleaseAsset{
		{Name: "new-api-" + tag},
		{Name: "new-api-arm64-" + tag},
		{Name: "zeuszs-updater-linux-amd64-" + tag},
		{Name: "zeuszs-updater-linux-arm64-" + tag},
		{Name: "checksums-linux.txt"},
		{Name: "LICENSE"},
		{Name: "NOTICE"},
		{Name: "THIRD-PARTY-LICENSES.md"},
	}
}

func TestCheckZeusZSUpdateSelectsHighestSemanticRelease(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{
		{ID: 1, TagName: "v99.0.0", Name: "wrong project tag"},
		{ID: 2, TagName: "zeuszs-v99.0.0", Name: "draft", Draft: true},
		{ID: 3, TagName: "zeuszs-vinvalid", Name: "invalid"},
		{ID: 4, TagName: "zeuszs-v9.0.0-beta.1", Name: "beta", Prerelease: true},
		{ID: 7, TagName: "zeuszs-v10.0.0-rc.1", Name: "mislabelled prerelease"},
		{ID: 8, TagName: "zeuszs-vv11.0.0", Name: "double version prefix"},
		{ID: 9, TagName: "zeuszs-v12.0", Name: "incomplete semantic version"},
		{ID: 10, TagName: "zeuszs-v013.0.0", Name: "leading zero"},
		{ID: 5, TagName: "zeuszs-v0.2.10", Name: "patch"},
		{ID: 6, TagName: "zeuszs-v0.10.0", Name: "latest", HTMLURL: "https://example.test/latest"},
	})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "zeuszs-v0.2.9" }
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig { return zeusZSUpdaterConfig{} }

	result, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, zeusZSRepository, result.Repository)
	assert.Equal(t, "zeuszs-v0.2.9", result.CurrentVersion)
	assert.Equal(t, int64(6), result.LatestRelease.ID)
	assert.Equal(t, "zeuszs-v0.10.0", result.LatestRelease.TagName)
	assert.True(t, result.UpdateAvailable)
	assert.False(t, result.UpdaterConfigured)
}

func TestCheckZeusZSUpdateRejectsInvalidCurrentVersion(t *testing.T) {
	setupZeusZSUpdateTest(t)
	zeusZSCurrentVersion = func() string { return "development" }

	_, err := CheckZeusZSUpdate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrZeusZSCurrentVersionInvalid)
}

func TestCheckZeusZSUpdateSkipsReleaseUntilLinuxAssetsAreComplete(t *testing.T) {
	setupZeusZSUpdateTest(t)
	incompleteAssets := testZeusZSReleaseAssets("zeuszs-v0.5.0")
	incompleteAssets = incompleteAssets[:len(incompleteAssets)-1]
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{
		{ID: 5, TagName: "zeuszs-v0.5.0", Assets: incompleteAssets},
		{ID: 4, TagName: "zeuszs-v0.4.0"},
	})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "zeuszs-v0.4.0" }
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig { return zeusZSUpdaterConfig{} }

	result, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "zeuszs-v0.4.0", result.LatestRelease.TagName)
	assert.False(t, result.UpdateAvailable)
}

func TestZeusZSUpdaterEndpointsRejectWeakToken(t *testing.T) {
	_, _, _, err := zeusZSUpdaterEndpoints(zeusZSUpdaterConfig{
		TriggerURL:   "http://127.0.0.1:8080/v1/update",
		TriggerToken: "short-token",
	})

	assert.ErrorIs(t, err, ErrZeusZSUpdaterNotConfigured)
}

func TestCheckZeusZSUpdateCachesRemoteReleaseOnly(t *testing.T) {
	setupZeusZSUpdateTest(t)
	var releaseRequests atomic.Int32
	releaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		releaseRequests.Add(1)
		payload, err := common.Marshal([]githubZeusZSRelease{{
			TagName: "zeuszs-v0.4.0",
			Assets:  testZeusZSReleaseAssets("zeuszs-v0.4.0"),
		}})
		require.NoError(t, err)
		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	t.Cleanup(releaseServer.Close)
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig { return zeusZSUpdaterConfig{} }
	now := time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC)
	zeusZSNow = func() time.Time { return now }

	zeusZSCurrentVersion = func() string { return "v0.3.0" }
	first, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	zeusZSCurrentVersion = func() string { return "v0.4.0" }
	second, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)

	assert.True(t, first.UpdateAvailable)
	assert.False(t, second.UpdateAvailable, "current version must not be cached with the release")
	assert.EqualValues(t, 1, releaseRequests.Load())

	now = now.Add(zeusZSReleaseCacheTTL + time.Second)
	_, err = CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 2, releaseRequests.Load())
}

func TestCheckZeusZSUpdateIncludesUpdaterStatusWithoutCachingIt(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v0.4.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	var statusRequests atomic.Int32
	updaterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/status", r.URL.Path)
		assert.Equal(t, "Bearer "+testZeusZSUpdaterToken, r.Header.Get("Authorization"))
		count := statusRequests.Add(1)
		payload, err := common.Marshal(ZeusZSUpdaterStatus{
			Status: "running",
			Tag:    "zeuszs-v0.4.0",
			Step:   "step-" + string(rune('0'+count)),
		})
		require.NoError(t, err)
		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	t.Cleanup(updaterServer.Close)
	zeusZSUpdateHTTPClient = updaterServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: updaterServer.URL + "/v1/update", TriggerToken: testZeusZSUpdaterToken}
	}

	first, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	second, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)

	assert.True(t, first.UpdaterConfigured)
	assert.True(t, first.UpdaterReachable)
	require.NotNil(t, first.UpdaterStatus)
	assert.Equal(t, "step-1", first.UpdaterStatus.Step)
	require.NotNil(t, second.UpdaterStatus)
	assert.Equal(t, "step-2", second.UpdaterStatus.Step, "updater status must not be cached")
	assert.EqualValues(t, 2, statusRequests.Load())
}

func TestCheckZeusZSUpdateSurvivesUnavailableUpdater(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v0.4.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	unavailableUpdater := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	updaterURL := unavailableUpdater.URL
	updaterClient := unavailableUpdater.Client()
	unavailableUpdater.Close()
	zeusZSUpdateHTTPClient = updaterClient
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: updaterURL + "/v1/update", TriggerToken: testZeusZSUpdaterToken}
	}

	result, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.True(t, result.UpdaterConfigured)
	assert.False(t, result.UpdaterReachable)
	assert.Nil(t, result.UpdaterStatus)
}

func TestTriggerZeusZSUpdatePostsServerSelectedTag(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{
		ID: 21, TagName: "zeuszs-v0.4.0", Name: "v0.4.0",
	}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	triggerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer "+testZeusZSUpdaterToken, r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var payload zeusZSUpdateTriggerPayload
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		assert.Equal(t, "zeuszs-v0.4.0", payload.Tag)
		response, err := common.Marshal(ZeusZSUpdaterStatus{Status: "running", Tag: payload.Tag, Step: "queued"})
		require.NoError(t, err)
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write(response)
		assert.NoError(t, err)
	}))
	t.Cleanup(triggerServer.Close)
	zeusZSUpdateHTTPClient = triggerServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: triggerServer.URL, TriggerToken: testZeusZSUpdaterToken}
	}

	result, err := TriggerZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "zeuszs-v0.4.0", result.LatestRelease.TagName)
	assert.True(t, result.UpdateAvailable)
	assert.True(t, result.UpdaterConfigured)
	assert.True(t, result.UpdaterReachable)
	require.NotNil(t, result.UpdaterStatus)
	assert.Equal(t, "queued", result.UpdaterStatus.Step)
	assert.NotEmpty(t, result.TriggeredAt)
}

func TestTriggerZeusZSUpdateUsesConfiguredUnixSocket(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v1.0.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "zeuszs-v0.9.0" }

	socketDir, err := os.MkdirTemp("/tmp", "zeuszs-update-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(socketDir)
	})
	socketPath := filepath.Join(socketDir, "updater.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "unix", r.Host)
		assert.Equal(t, "Bearer "+testZeusZSUpdaterToken, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/update":
			assert.Equal(t, http.MethodPost, r.Method)
			var payload zeusZSUpdateTriggerPayload
			require.NoError(t, common.DecodeJson(r.Body, &payload))
			assert.Equal(t, "zeuszs-v1.0.0", payload.Tag)
			w.WriteHeader(http.StatusAccepted)
		case "/v1/status":
			assert.Equal(t, http.MethodGet, r.Method)
			payload, marshalErr := common.Marshal(ZeusZSUpdaterStatus{Status: "running", Tag: "zeuszs-v1.0.0", Step: "build-image"})
			require.NoError(t, marshalErr)
			_, writeErr := w.Write(payload)
			assert.NoError(t, writeErr)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
	})
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{
			TriggerURL:    "https://ignored.example/update",
			TriggerToken:  testZeusZSUpdaterToken,
			TriggerSocket: socketPath,
		}
	}

	result, err := TriggerZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "zeuszs-v1.0.0", result.LatestRelease.TagName)
	status, err := CheckZeusZSUpdate(context.Background())
	require.NoError(t, err)
	assert.True(t, status.UpdaterReachable)
	require.NotNil(t, status.UpdaterStatus)
	assert.Equal(t, "build-image", status.UpdaterStatus.Step)
}

func TestTriggerZeusZSUpdateDoesNotCallUpdaterWithoutNewRelease(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v0.3.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	var triggerCalled atomic.Bool
	triggerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		triggerCalled.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(triggerServer.Close)
	zeusZSUpdateHTTPClient = triggerServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: triggerServer.URL, TriggerToken: testZeusZSUpdaterToken}
	}

	_, err := TriggerZeusZSUpdate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrZeusZSNoUpdateAvailable)
	assert.False(t, triggerCalled.Load())
}

func TestTriggerZeusZSUpdateRejectsConcurrentRequest(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v0.4.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	triggerEntered := make(chan struct{})
	releaseTrigger := make(chan struct{})
	triggerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(triggerEntered)
		<-releaseTrigger
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(triggerServer.Close)
	zeusZSUpdateHTTPClient = triggerServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: triggerServer.URL, TriggerToken: testZeusZSUpdaterToken}
	}

	firstResult := make(chan error, 1)
	go func() {
		_, err := TriggerZeusZSUpdate(context.Background())
		firstResult <- err
	}()
	select {
	case <-triggerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first update trigger did not reach updater")
	}

	_, err := TriggerZeusZSUpdate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrZeusZSUpdateInProgress)
	close(releaseTrigger)
	require.NoError(t, <-firstResult)
}

func TestTriggerZeusZSUpdateRequiresAcceptedResponse(t *testing.T) {
	setupZeusZSUpdateTest(t)
	releaseServer := zeusZSReleaseServer(t, []githubZeusZSRelease{{TagName: "zeuszs-v0.4.0"}})
	zeusZSReleaseAPIURL = releaseServer.URL
	zeusZSReleaseHTTPClient = releaseServer.Client()
	zeusZSCurrentVersion = func() string { return "v0.3.0" }

	triggerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(triggerServer.Close)
	zeusZSUpdateHTTPClient = triggerServer.Client()
	zeusZSUpdaterConfigLoader = func() zeusZSUpdaterConfig {
		return zeusZSUpdaterConfig{TriggerURL: triggerServer.URL, TriggerToken: testZeusZSUpdaterToken}
	}

	_, err := TriggerZeusZSUpdate(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZeusZSUpdateTriggerFailed))
}
