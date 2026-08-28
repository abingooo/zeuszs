package main

import (
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const maxRequestBodyBytes = 4096

type updateStatus struct {
	Status     string     `json:"status"`
	Tag        string     `json:"tag,omitempty"`
	Step       string     `json:"step,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type updateExecutor interface {
	Execute(tag string, reportStep func(string)) error
}

type statusStore interface {
	Load() (updateStatus, error)
	Save(updateStatus) error
}

type updateManager struct {
	mu        sync.RWMutex
	status    updateStatus
	executor  updateExecutor
	store     statusStore
	logger    *log.Logger
	accepting bool
	done      chan struct{}
}

func newUpdateManager(executor updateExecutor, store statusStore, logger *log.Logger) *updateManager {
	status := updateStatus{Status: "idle", UpdatedAt: time.Now().UTC()}
	if stored, err := store.Load(); err == nil {
		status = stored
		if status.Status == "running" {
			now := time.Now().UTC()
			status.Status = "failed"
			status.Step = "interrupted"
			status.Error = "updater restarted before the update completed"
			status.FinishedAt = &now
			status.UpdatedAt = now
			_ = store.Save(status)
		}
	} else if !errors.Is(err, errStatusNotFound) {
		logger.Printf("could not load updater state: %v", err)
	}
	return &updateManager{status: status, executor: executor, store: store, logger: logger, accepting: true}
}

func (manager *updateManager) Start(tag string) (updateStatus, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.accepting || manager.status.Status == "running" {
		return manager.status, false
	}

	now := time.Now().UTC()
	manager.status = updateStatus{
		Status:    "running",
		Tag:       tag,
		Step:      "queued",
		StartedAt: &now,
		UpdatedAt: now,
	}
	done := make(chan struct{})
	manager.done = done
	manager.saveLocked()
	go manager.run(tag, done)
	return manager.status, true
}

func (manager *updateManager) Status() updateStatus {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.status
}

func (manager *updateManager) run(tag string, done chan struct{}) {
	defer close(done)
	manager.logger.Printf("update started tag=%s", tag)
	err := manager.executor.Execute(tag, manager.reportStep)

	manager.mu.Lock()
	defer manager.mu.Unlock()
	now := time.Now().UTC()
	manager.status.FinishedAt = &now
	manager.status.UpdatedAt = now
	if err != nil {
		manager.status.Status = "failed"
		manager.status.Error = err.Error()
		manager.logger.Printf("update failed tag=%s step=%s error=%v", tag, manager.status.Step, err)
	} else {
		manager.status.Status = "succeeded"
		manager.status.Step = "complete"
		manager.status.Error = ""
		manager.logger.Printf("update succeeded tag=%s", tag)
	}
	manager.saveLocked()
}

func (manager *updateManager) StopAndWait() {
	manager.mu.Lock()
	manager.accepting = false
	done := manager.done
	running := manager.status.Status == "running"
	manager.mu.Unlock()
	if running && done != nil {
		<-done
	}
}

func (manager *updateManager) reportStep(step string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.status.Step = step
	manager.status.UpdatedAt = time.Now().UTC()
	manager.saveLocked()
	manager.logger.Printf("update step tag=%s step=%s", manager.status.Tag, step)
}

func (manager *updateManager) saveLocked() {
	if err := manager.store.Save(manager.status); err != nil {
		manager.logger.Printf("could not save updater state: %v", err)
	}
}

type apiServer struct {
	token   string
	manager *updateManager
}

func (server *apiServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/update", server.authorize(server.handleUpdate))
	mux.HandleFunc("/v1/status", server.authorize(server.handleStatus))
	return mux
}

func (server *apiServer) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		expected := "Bearer " + server.token
		provided := request.Header.Get("Authorization")
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(writer, request)
	}
}

func (server *apiServer) handleUpdate(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		writeJSON(writer, http.StatusUnsupportedMediaType, map[string]string{"error": "content type must be application/json"})
		return
	}

	tag, err := decodeUpdateRequest(request.Body)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	status, started := server.manager.Start(tag)
	if !started {
		writeJSON(writer, http.StatusConflict, status)
		return
	}
	writeJSON(writer, http.StatusAccepted, status)
}

func (server *apiServer) handleStatus(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(writer, http.StatusOK, server.manager.Status())
}

func decodeUpdateRequest(body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxRequestBodyBytes+1))
	if err != nil {
		return "", errors.New("could not read request")
	}
	if len(data) > maxRequestBodyBytes {
		return "", errors.New("request body is too large")
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return "", errors.New("invalid JSON body")
	}
	if len(payload) != 1 {
		return "", errors.New("request must contain only tag")
	}
	tag, ok := payload["tag"].(string)
	if !ok || !validReleaseTag(tag) {
		return "", errors.New("tag must match zeuszs-v<semver>")
	}
	return tag, nil
}

func writeJSON(writer http.ResponseWriter, statusCode int, value any) {
	payload, err := common.Marshal(value)
	if err != nil {
		http.Error(writer, "could not encode response", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_, _ = writer.Write(payload)
}
