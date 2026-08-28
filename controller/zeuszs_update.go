package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var (
	zeusZSCheckUpdate   = service.CheckZeusZSUpdate
	zeusZSTriggerUpdate = service.TriggerZeusZSUpdate
)

func GetZeusZSUpdate(c *gin.Context) {
	result, err := zeusZSCheckUpdate(c.Request.Context())
	if err != nil {
		zeusZSUpdateError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func TriggerZeusZSUpdate(c *gin.Context) {
	if _, ok := middleware.GetSessionAuthIdentity(c); !ok {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"code":    "AUTH_SESSION_REQUIRED",
			"message": "a dashboard login session is required",
			"data":    nil,
		})
		return
	}
	if c.Request.Body != nil {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"code":    "ZEUSZS_UPDATE_REQUEST_INVALID",
				"message": "unable to read update request",
				"data":    nil,
			})
			return
		}
		if len(body) != 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"code":    "ZEUSZS_UPDATE_REQUEST_BODY_NOT_ALLOWED",
				"message": "the update trigger does not accept request parameters",
				"data":    nil,
			})
			return
		}
	}

	actorID := c.GetInt("id")
	result, err := zeusZSTriggerUpdate(c.Request.Context())
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("ZeusZS update trigger rejected actor_user_id=%d error=%q", actorID, err.Error()))
		zeusZSUpdateError(c, err)
		return
	}
	recordManageAudit(c, "zeuszs.update_trigger", map[string]interface{}{
		"target_tag": result.LatestRelease.TagName,
	})
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("ZeusZS update trigger accepted actor_user_id=%d target_tag=%q", actorID, result.LatestRelease.TagName))
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "ZeusZS update trigger accepted",
		"data":    result,
	})
}

func zeusZSUpdateError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "ZEUSZS_UPDATE_INTERNAL_ERROR"
	message := "ZeusZS update operation failed"
	switch {
	case errors.Is(err, service.ErrZeusZSReleaseNotFound):
		status = http.StatusNotFound
		code = "ZEUSZS_RELEASE_NOT_FOUND"
		message = "no ZeusZS release was found"
	case errors.Is(err, service.ErrZeusZSCurrentVersionInvalid):
		code = "ZEUSZS_CURRENT_VERSION_INVALID"
		message = "the current ZeusZS version is invalid"
	case errors.Is(err, service.ErrZeusZSReleaseCheckFailed):
		status = http.StatusBadGateway
		code = "ZEUSZS_RELEASE_CHECK_FAILED"
		message = "unable to check ZeusZS releases"
	case errors.Is(err, service.ErrZeusZSUpdaterNotConfigured):
		status = http.StatusServiceUnavailable
		code = "ZEUSZS_UPDATER_NOT_CONFIGURED"
		message = "the ZeusZS updater is not configured"
	case errors.Is(err, service.ErrZeusZSUpdateInProgress):
		status = http.StatusConflict
		code = "ZEUSZS_UPDATE_IN_PROGRESS"
		message = "a ZeusZS update trigger is already in progress"
	case errors.Is(err, service.ErrZeusZSNoUpdateAvailable):
		status = http.StatusConflict
		code = "ZEUSZS_NO_UPDATE_AVAILABLE"
		message = "ZeusZS is already up to date"
	case errors.Is(err, service.ErrZeusZSUpdateTriggerFailed):
		status = http.StatusBadGateway
		code = "ZEUSZS_UPDATE_TRIGGER_FAILED"
		message = "the ZeusZS updater did not accept the request"
	}
	c.JSON(status, gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"data":    nil,
	})
}
