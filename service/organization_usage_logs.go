package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/custom_setting"

	"gorm.io/gorm"
)

type ListOrganizationUsageLogsParams struct {
	Offset         int
	Limit          int
	OrganizationID int
	ActorUserID    int
	StartTimestamp int64
	EndTimestamp   int64
	Action         string
	TargetType     string
	TargetID       string
	RequestID      string
}

type OrganizationUsageLogView struct {
	ID                int64           `json:"id"`
	OrganizationID    int             `json:"organization_id,omitempty"`
	OrganizationName  string          `json:"organization_name"`
	ActorUserID       int             `json:"actor_user_id,omitempty"`
	ActorUsername     string          `json:"actor_username"`
	InitiatorUserID   *int            `json:"initiator_user_id,omitempty"`
	InitiatorUsername string          `json:"initiator_username,omitempty"`
	Action            string          `json:"action"`
	TargetType        string          `json:"target_type"`
	TargetID          string          `json:"target_id,omitempty"`
	TargetName        string          `json:"target_name,omitempty"`
	RequestID         string          `json:"request_id"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         int64           `json:"created_at"`
}

type OrganizationUsageLogListResult struct {
	Items []OrganizationUsageLogView `json:"items"`
	Total int64                      `json:"total"`
}

type organizationUsageLogViewer struct {
	UserID         int
	OrganizationID int
	Role           model.OrganizationRole
	PlatformWide   bool
	IDsVisible     bool
}

func sanitizeOrganizationMemberUsageLogMetadata(action string, metadata map[string]interface{}) (json.RawMessage, error) {
	allowedKeys := []string{}
	switch action {
	case "organization.member.join", "organization.member.provision":
		allowedKeys = []string{"organization_role", "role"}
	case "organization.member.role.update":
		allowedKeys = []string{"from_role", "to_role"}
	case "organization.member.status.update", "organization.status.update":
		allowedKeys = []string{"from", "to"}
	case "organization.member.limit.update":
		allowedKeys = []string{"from", "to"}
	case "organization.member.tokens.disable":
		allowedKeys = []string{"disabled_token_count"}
	case "organization.quota.allocate", "organization.quota.recover":
		allowedKeys = []string{
			"user_quota_delta",
			"user_quota_after",
			"recoverable_quota_delta",
			"recoverable_quota_after",
		}
	}

	sanitized := make(map[string]interface{}, len(allowedKeys)+1)
	for _, key := range allowedKeys {
		if value, ok := metadata[key]; ok {
			sanitized[key] = value
		}
	}
	if action == "organization.fund.credit" {
		if amount, ok := metadata["pool_quota_delta"]; ok {
			sanitized["amount"] = amount
		}
	}
	encoded, err := common.Marshal(sanitized)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func sanitizeOrganizationUsageLogInternalIDValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(typed))
		for key, nestedValue := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(normalizedKey, "user_id") ||
				strings.Contains(normalizedKey, "organization_id") ||
				normalizedKey == "source_id" || normalizedKey == "target_id" ||
				normalizedKey == "idempotency_key" {
				continue
			}
			sanitized[key] = sanitizeOrganizationUsageLogInternalIDValue(nestedValue)
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, len(typed))
		for i := range typed {
			sanitized[i] = sanitizeOrganizationUsageLogInternalIDValue(typed[i])
		}
		return sanitized
	default:
		return value
	}
}

func sanitizeOrganizationUsageLogInternalIDs(metadata map[string]interface{}) (json.RawMessage, error) {
	encoded, err := common.Marshal(sanitizeOrganizationUsageLogInternalIDValue(metadata))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func loadOrganizationUsageLogViewerTx(tx *gorm.DB, userID int) (*organizationUsageLogViewer, error) {
	if tx == nil || userID <= 0 {
		return nil, ErrOrganizationIdentityInvalid
	}
	var user model.User
	if err := tx.Select(
		"id", "role", "status", "organization_id", "organization_role", "organization_status",
	).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationIdentityInvalid
		}
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrOrganizationActionForbidden
	}
	if user.Role == common.RoleAdminUser || user.Role == common.RoleRootUser {
		return &organizationUsageLogViewer{
			UserID: user.Id, PlatformWide: true, IDsVisible: true,
		}, nil
	}
	if user.Role != common.RoleCommonUser || user.OrganizationId <= 0 ||
		user.OrganizationStatus != model.OrganizationMemberStatusActive {
		return nil, ErrOrganizationIdentityInvalid
	}
	if user.OrganizationRole != model.OrganizationRoleOwner &&
		user.OrganizationRole != model.OrganizationRoleAdmin &&
		user.OrganizationRole != model.OrganizationRoleMember {
		return nil, ErrOrganizationIdentityInvalid
	}

	var organization model.Organization
	if err := tx.Select("id", "status", "owner_user_id", "system_key").Where("id = ?", user.OrganizationId).First(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrganizationIdentityInvalid
		}
		return nil, err
	}
	if organization.Status != model.OrganizationStatusActive {
		return nil, ErrOrganizationInactive
	}
	if organization.SystemKey != nil && *organization.SystemKey == model.DefaultOrganizationSystemKey {
		return nil, ErrOrganizationActionForbidden
	}
	if user.OrganizationRole == model.OrganizationRoleOwner && organization.OwnerUserId != user.Id {
		return nil, ErrOrganizationIdentityInvalid
	}
	return &organizationUsageLogViewer{
		UserID: user.Id, OrganizationID: organization.Id, Role: user.OrganizationRole,
		IDsVisible: custom_setting.IsIDVisibilityEnabled(),
	}, nil
}

// ListOrganizationUsageLogs exposes the organization audit stream according to
// the viewer's current server-side roles. Platform administrators see all
// organizations, tenant Owner/Admin roles see their organization, and Members
// see only events in their organization that directly name them.
func ListOrganizationUsageLogs(viewerUserID int, params ListOrganizationUsageLogsParams) (*OrganizationUsageLogListResult, error) {
	if params.Offset < 0 {
		params.Offset = 0
	}
	if params.Limit <= 0 {
		params.Limit = common.ItemsPerPage
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	params.Action = strings.TrimSpace(params.Action)
	params.TargetType = strings.TrimSpace(params.TargetType)
	params.TargetID = strings.TrimSpace(params.TargetID)
	params.RequestID = strings.TrimSpace(params.RequestID)
	if viewerUserID <= 0 || params.OrganizationID < 0 || params.ActorUserID < 0 ||
		params.StartTimestamp < 0 || params.EndTimestamp < 0 ||
		(params.StartTimestamp > 0 && params.EndTimestamp > 0 && params.StartTimestamp > params.EndTimestamp) ||
		len(params.Action) > 64 || len(params.TargetType) > 32 || len(params.TargetID) > 128 ||
		len(params.RequestID) > 64 || model.DB == nil {
		return nil, ErrTenantOrganizationRequestInvalid
	}

	result := &OrganizationUsageLogListResult{Items: make([]OrganizationUsageLogView, 0)}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		viewer, err := loadOrganizationUsageLogViewerTx(tx, viewerUserID)
		if err != nil {
			return err
		}

		query := tx.Model(&model.OrganizationAuditEvent{}).
			Where("action NOT LIKE ?", "organization.wallet.%")
		if viewer.PlatformWide {
			if params.OrganizationID > 0 {
				query = query.Where("organization_id = ?", params.OrganizationID)
			}
		} else {
			if params.OrganizationID > 0 && params.OrganizationID != viewer.OrganizationID {
				return gorm.ErrRecordNotFound
			}
			query = query.Where("organization_id = ?", viewer.OrganizationID)
			if viewer.Role == model.OrganizationRoleMember {
				query = query.Where(
					"(actor_user_id = ? OR initiator_user_id = ? OR (target_type = ? AND target_id = ?))",
					viewer.UserID,
					viewer.UserID,
					"user",
					strconv.Itoa(viewer.UserID),
				)
			}
		}
		if params.ActorUserID > 0 {
			query = query.Where("actor_user_id = ?", params.ActorUserID)
		}
		if params.StartTimestamp > 0 {
			query = query.Where("created_at >= ?", params.StartTimestamp)
		}
		if params.EndTimestamp > 0 {
			query = query.Where("created_at <= ?", params.EndTimestamp)
		}
		if params.Action != "" {
			query = query.Where("action = ?", params.Action)
		}
		if params.TargetType != "" {
			query = query.Where("target_type = ?", params.TargetType)
		}
		if params.TargetID != "" {
			query = query.Where("target_id = ?", params.TargetID)
		}
		if params.RequestID != "" {
			query = query.Where("request_id = ?", params.RequestID)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return err
		}

		var events []model.OrganizationAuditEvent
		if err := query.Order("created_at desc, id desc").Offset(params.Offset).Limit(params.Limit).Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		organizationIDs := make(map[int]struct{})
		userIDs := make(map[int]struct{})
		for i := range events {
			event := &events[i]
			organizationIDs[event.OrganizationId] = struct{}{}
			if event.ActorUserId > 0 {
				userIDs[event.ActorUserId] = struct{}{}
			}
			if event.InitiatorUserId != nil && *event.InitiatorUserId > 0 {
				userIDs[*event.InitiatorUserId] = struct{}{}
			}
			if event.TargetType == "user" {
				if targetUserID, parseErr := strconv.Atoi(event.TargetId); parseErr == nil && targetUserID > 0 {
					userIDs[targetUserID] = struct{}{}
				}
			}
		}

		organizationIDList := make([]int, 0, len(organizationIDs))
		for organizationID := range organizationIDs {
			organizationIDList = append(organizationIDList, organizationID)
		}
		var organizations []model.Organization
		if err := tx.Select("id", "name").Where("id IN ?", organizationIDList).Find(&organizations).Error; err != nil {
			return err
		}
		organizationNames := make(map[int]string, len(organizations))
		for i := range organizations {
			organizationNames[organizations[i].Id] = organizations[i].Name
		}

		usernames := make(map[int]string, len(userIDs))
		if len(userIDs) > 0 {
			userIDList := make([]int, 0, len(userIDs))
			for userID := range userIDs {
				userIDList = append(userIDList, userID)
			}
			var users []model.User
			if err := tx.Unscoped().Select("id", "username").Where("id IN ?", userIDList).Find(&users).Error; err != nil {
				return err
			}
			for i := range users {
				usernames[users[i].Id] = users[i].Username
			}
		}

		result.Items = make([]OrganizationUsageLogView, 0, len(events))
		for i := range events {
			event := &events[i]
			metadataText := strings.TrimSpace(event.Metadata)
			if metadataText == "" {
				metadataText = "{}"
			}
			metadata := json.RawMessage(metadataText)
			var validated map[string]interface{}
			if err := common.Unmarshal(metadata, &validated); err != nil {
				common.SysError("invalid organization audit metadata for event " + strconv.FormatInt(event.Id, 10) + ": " + err.Error())
				validated = map[string]interface{}{}
				metadata = json.RawMessage("{}")
			}
			if viewer.Role == model.OrganizationRoleMember {
				metadata, err = sanitizeOrganizationMemberUsageLogMetadata(event.Action, validated)
				if err != nil {
					return err
				}
			} else if !viewer.PlatformWide && !viewer.IDsVisible {
				metadata, err = sanitizeOrganizationUsageLogInternalIDs(validated)
				if err != nil {
					return err
				}
			}
			view := OrganizationUsageLogView{
				ID:               event.Id,
				OrganizationName: organizationNames[event.OrganizationId],
				ActorUsername:    usernames[event.ActorUserId],
				Action:           event.Action,
				TargetType:       event.TargetType,
				RequestID:        event.RequestId, Metadata: metadata, CreatedAt: event.CreatedAt,
			}
			if event.InitiatorUserId != nil {
				view.InitiatorUsername = usernames[*event.InitiatorUserId]
			}
			if viewer.IDsVisible {
				view.OrganizationID = event.OrganizationId
				view.ActorUserID = event.ActorUserId
				view.InitiatorUserID = event.InitiatorUserId
				view.TargetID = event.TargetId
			}
			switch event.TargetType {
			case "user":
				if targetUserID, parseErr := strconv.Atoi(event.TargetId); parseErr == nil {
					view.TargetName = usernames[targetUserID]
				}
			case "organization":
				if targetOrganizationID, parseErr := strconv.Atoi(event.TargetId); parseErr == nil {
					view.TargetName = organizationNames[targetOrganizationID]
				}
			}
			result.Items = append(result.Items, view)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
