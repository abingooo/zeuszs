package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupZeusZSUpdateControllerTest(t *testing.T) {
	t.Helper()
	previousCheck := zeusZSCheckUpdate
	previousTrigger := zeusZSTriggerUpdate
	t.Cleanup(func() {
		zeusZSCheckUpdate = previousCheck
		zeusZSTriggerUpdate = previousTrigger
	})
}

func setZeusZSUpdateTestSession(c *gin.Context) {
	c.Set("id", 1)
	c.Set("session_id", "zeuszs-update-session")
	c.Set("auth_version", int64(1))
	c.Set("session_version", int64(1))
}

func TestTriggerZeusZSUpdateRejectsRequestParameters(t *testing.T) {
	setupZeusZSUpdateControllerTest(t)
	gin.SetMode(gin.TestMode)
	called := false
	zeusZSTriggerUpdate = func(context.Context) (service.ZeusZSUpdateTriggerResult, error) {
		called = true
		return service.ZeusZSUpdateTriggerResult{}, nil
	}
	engine := gin.New()
	engine.POST("/api/zeuszs/update", func(c *gin.Context) {
		setZeusZSUpdateTestSession(c)
		TriggerZeusZSUpdate(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/zeuszs/update", bytes.NewBufferString(`{"target_tag":"zeuszs-v99.0.0"}`))
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"ZEUSZS_UPDATE_REQUEST_BODY_NOT_ALLOWED"`)
	assert.False(t, called)
}

func TestTriggerZeusZSUpdateAcceptsEmptyBody(t *testing.T) {
	setupZeusZSUpdateControllerTest(t)
	gin.SetMode(gin.TestMode)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	organization := model.Organization{
		Name:          "ZeusZS Update Audit Organization",
		Status:        model.OrganizationStatusActive,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	actor := model.User{
		Username:           "zeuszs-update-auditor",
		Role:               common.RoleAdminUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationStatus: model.OrganizationMemberStatusActive,
		AffCode:            "zeuszs-update-auditor-aff",
	}
	require.NoError(t, db.Create(&actor).Error)
	zeusZSTriggerUpdate = func(context.Context) (service.ZeusZSUpdateTriggerResult, error) {
		return service.ZeusZSUpdateTriggerResult{
			ZeusZSUpdateCheck: service.ZeusZSUpdateCheck{
				LatestRelease: service.ZeusZSRelease{TagName: "zeuszs-v0.4.0"},
			},
			TriggeredAt: "2026-08-28T00:00:00Z",
		}, nil
	}
	engine := gin.New()
	engine.POST("/api/zeuszs/update", func(c *gin.Context) {
		c.Set("id", actor.Id)
		c.Set("username", actor.Username)
		c.Set("role", actor.Role)
		c.Set("session_id", "zeuszs-update-session")
		c.Set("auth_version", int64(1))
		c.Set("session_version", int64(1))
		TriggerZeusZSUpdate(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/zeuszs/update", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"tag_name":"zeuszs-v0.4.0"`)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	assert.Equal(t, "Triggered ZeusZS update to zeuszs-v0.4.0", auditLog.Content)
	var auditData struct {
		Operation struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, "zeuszs.update_trigger", auditData.Operation.Action)
	assert.Equal(t, "zeuszs-v0.4.0", auditData.Operation.Params["target_tag"])
}

func TestTriggerZeusZSUpdateRejectsNonSessionCredential(t *testing.T) {
	setupZeusZSUpdateControllerTest(t)
	gin.SetMode(gin.TestMode)
	called := false
	zeusZSTriggerUpdate = func(context.Context) (service.ZeusZSUpdateTriggerResult, error) {
		called = true
		return service.ZeusZSUpdateTriggerResult{}, nil
	}
	engine := gin.New()
	engine.POST("/api/zeuszs/update", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("use_access_token", true)
		TriggerZeusZSUpdate(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/zeuszs/update", nil)
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Contains(t, response.Body.String(), `"code":"AUTH_SESSION_REQUIRED"`)
	assert.False(t, called)
}

func TestTriggerZeusZSUpdateMapsServiceErrors(t *testing.T) {
	setupZeusZSUpdateControllerTest(t)
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "release not found", err: service.ErrZeusZSReleaseNotFound, status: http.StatusNotFound, code: "ZEUSZS_RELEASE_NOT_FOUND"},
		{name: "invalid current version", err: service.ErrZeusZSCurrentVersionInvalid, status: http.StatusInternalServerError, code: "ZEUSZS_CURRENT_VERSION_INVALID"},
		{name: "release check failed", err: service.ErrZeusZSReleaseCheckFailed, status: http.StatusBadGateway, code: "ZEUSZS_RELEASE_CHECK_FAILED"},
		{name: "updater not configured", err: service.ErrZeusZSUpdaterNotConfigured, status: http.StatusServiceUnavailable, code: "ZEUSZS_UPDATER_NOT_CONFIGURED"},
		{name: "update in progress", err: service.ErrZeusZSUpdateInProgress, status: http.StatusConflict, code: "ZEUSZS_UPDATE_IN_PROGRESS"},
		{name: "no update available", err: service.ErrZeusZSNoUpdateAvailable, status: http.StatusConflict, code: "ZEUSZS_NO_UPDATE_AVAILABLE"},
		{name: "trigger failed", err: service.ErrZeusZSUpdateTriggerFailed, status: http.StatusBadGateway, code: "ZEUSZS_UPDATE_TRIGGER_FAILED"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			zeusZSTriggerUpdate = func(context.Context) (service.ZeusZSUpdateTriggerResult, error) {
				return service.ZeusZSUpdateTriggerResult{}, testCase.err
			}
			engine := gin.New()
			engine.POST("/api/zeuszs/update", func(c *gin.Context) {
				setZeusZSUpdateTestSession(c)
				TriggerZeusZSUpdate(c)
			})
			request := httptest.NewRequest(http.MethodPost, "/api/zeuszs/update", nil)
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			require.Equal(t, testCase.status, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"code":"`+testCase.code+`"`)
			assert.Contains(t, recorder.Body.String(), `"data":null`)
		})
	}
}
