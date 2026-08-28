package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStatusStore struct {
	mu     sync.Mutex
	status updateStatus
	exists bool
}

func (store *memoryStatusStore) Load() (updateStatus, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.exists {
		return updateStatus{}, errStatusNotFound
	}
	return store.status, nil
}

func (store *memoryStatusStore) Save(status updateStatus) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.status = status
	store.exists = true
	return nil
}

type immediateExecutor struct{}

func (immediateExecutor) Execute(_ string, reportStep func(string)) error {
	reportStep("executed")
	return nil
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (executor *blockingExecutor) Execute(_ string, reportStep func(string)) error {
	reportStep("blocked")
	close(executor.started)
	<-executor.release
	return nil
}

func newTestAPIServer(executor updateExecutor) (*apiServer, *updateManager) {
	manager := newUpdateManager(executor, &memoryStatusStore{}, log.New(io.Discard, "", 0))
	return &apiServer{token: "test-token", manager: manager}, manager
}

func TestAPIServerRequiresBearerToken(t *testing.T) {
	server, _ := newTestAPIServer(immediateExecutor{})

	for _, authorization := range []string{"", "Bearer wrong-token", "Basic test-token"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()

		server.Handler().ServeHTTP(response, request)

		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.Equal(t, "Bearer", response.Header().Get("WWW-Authenticate"))
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestDecodeUpdateRequestRejectsAnythingExceptOneValidTag(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{name: "stable", body: `{"tag":"zeuszs-v0.5.0"}`, want: "zeuszs-v0.5.0", ok: true},
		{name: "prerelease", body: `{"tag":"zeuszs-v1.2.3-rc.1"}`},
		{name: "build metadata", body: `{"tag":"zeuszs-v1.2.3+build.1"}`},
		{name: "wrong prefix", body: `{"tag":"v0.5.0"}`},
		{name: "leading zero", body: `{"tag":"zeuszs-v01.5.0"}`},
		{name: "invalid prerelease", body: `{"tag":"zeuszs-v1.2.3-01"}`},
		{name: "shell text", body: `{"tag":"zeuszs-v1.2.3;reboot"}`},
		{name: "extra property", body: `{"tag":"zeuszs-v0.5.0","compose_dir":"/tmp"}`},
		{name: "trailing JSON", body: `{"tag":"zeuszs-v0.5.0"}{}`},
		{name: "wrong type", body: `{"tag":5}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag, err := decodeUpdateRequest(strings.NewReader(test.body))
			if test.ok {
				require.NoError(t, err)
				assert.Equal(t, test.want, tag)
				return
			}
			assert.Error(t, err)
		})
	}
}

func TestUpdateEndpointRejectsConcurrentUpdate(t *testing.T) {
	executor := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{})}
	server, manager := newTestAPIServer(executor)
	handler := server.Handler()

	first := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"tag":"zeuszs-v0.5.0"}`))
	first.Header.Set("Authorization", "Bearer test-token")
	first.Header.Set("Content-Type", "application/json")
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, first)
	require.Equal(t, http.StatusAccepted, firstResponse.Code)

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		require.FailNow(t, "update executor did not start")
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"tag":"zeuszs-v0.5.1"}`))
	second.Header.Set("Authorization", "Bearer test-token")
	second.Header.Set("Content-Type", "application/json")
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, second)
	assert.Equal(t, http.StatusConflict, secondResponse.Code)
	assert.Equal(t, "zeuszs-v0.5.0", manager.Status().Tag)

	close(executor.release)
	require.Eventually(t, func() bool {
		return manager.Status().Status == "succeeded"
	}, time.Second, time.Millisecond)
}
