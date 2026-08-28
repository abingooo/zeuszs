package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/custom_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createOrganizationUsageLogEvent(t *testing.T, event model.OrganizationAuditEvent) {
	t.Helper()
	if event.Metadata == "" {
		event.Metadata = `{"marker":"usage-log"}`
	}
	require.NoError(t, model.DB.Create(&event).Error)
}

func organizationUsageLogActions(items []OrganizationUsageLogView) map[string]OrganizationUsageLogView {
	byAction := make(map[string]OrganizationUsageLogView, len(items))
	for i := range items {
		byAction[items[i].Action] = items[i]
	}
	return byAction
}

func TestOrganizationUsageLogRoleMatrixAndTenantBoundaries(t *testing.T) {
	originalIDVisibility := custom_setting.IsIDVisibilityEnabled()
	custom_setting.SetIDVisibilityEnabled(false)
	t.Cleanup(func() {
		custom_setting.SetIDVisibilityEnabled(originalIDVisibility)
	})

	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", organization.Id).Update("name", "Alpha Lab").Error)
	organization.Name = "Alpha Lab"
	alphaOtherMember := createOrganizationManagementUser(t, db, "usage-alpha-other-member", common.RoleCommonUser, organization.Id, model.OrganizationRoleMember)

	otherOwner := createOrganizationManagementUser(t, db, "usage-beta-owner", common.RoleCommonUser, 0, "")
	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", otherOrganization.Id).Update("name", "Beta Lab").Error)
	otherOrganization.Name = "Beta Lab"
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id": otherOrganization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	otherOwner.OrganizationId = otherOrganization.Id
	otherOwner.OrganizationRole = model.OrganizationRoleOwner
	betaMember := createOrganizationManagementUser(t, db, "usage-beta-member", common.RoleCommonUser, otherOrganization.Id, model.OrganizationRoleMember)

	platformAdmin := createOrganizationManagementUser(t, db, "usage-platform-admin", common.RoleAdminUser, 0, "")
	platformRoot := createOrganizationManagementUser(t, db, "usage-platform-root", common.RoleRootUser, 0, "")
	memberID := member.Id

	events := []model.OrganizationAuditEvent{
		{
			OrganizationId: organization.Id, ActorUserId: owner.Id, Action: "organization.owner.action",
			TargetType: "organization", TargetId: strconv.Itoa(organization.Id), RequestId: "usage-owner",
			Metadata: `{"marker":"owner"}`, CreatedAt: 101,
		},
		{
			OrganizationId: organization.Id, ActorUserId: member.Id, Action: "organization.member.actor",
			TargetType: "organization", TargetId: strconv.Itoa(organization.Id), RequestId: "usage-member-actor",
			Metadata: `{"marker":"actor"}`, CreatedAt: 102,
		},
		{
			OrganizationId: organization.Id, ActorUserId: 0, InitiatorUserId: &memberID, Action: "organization.member.initiator",
			TargetType: "organization", TargetId: strconv.Itoa(organization.Id), RequestId: "usage-member-initiator",
			Metadata: `{"marker":"initiator"}`, CreatedAt: 103,
		},
		{
			OrganizationId: organization.Id, ActorUserId: admin.Id, Action: "organization.member.target",
			TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "usage-member-target",
			Metadata: `{"marker":"target"}`, CreatedAt: 104,
		},
		{
			OrganizationId: organization.Id, ActorUserId: admin.Id, Action: "organization.unrelated",
			TargetType: "user", TargetId: strconv.Itoa(alphaOtherMember.Id), RequestId: "usage-unrelated",
			Metadata: `{"marker":"unrelated"}`, CreatedAt: 105,
		},
		{
			OrganizationId: organization.Id, ActorUserId: alphaOtherMember.Id, Action: "organization.target.type.boundary",
			TargetType: "organization", TargetId: strconv.Itoa(member.Id), RequestId: "usage-target-type",
			Metadata: `{"marker":"target-type"}`, CreatedAt: 106,
		},
		{
			OrganizationId: otherOrganization.Id, ActorUserId: member.Id, Action: "organization.cross.actor",
			TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "usage-cross-actor",
			Metadata: `{"marker":"cross-actor"}`, CreatedAt: 107,
		},
		{
			OrganizationId: otherOrganization.Id, ActorUserId: otherOwner.Id, Action: "organization.beta.action",
			TargetType: "user", TargetId: strconv.Itoa(betaMember.Id), RequestId: "usage-beta",
			Metadata: `{"marker":"beta"}`, CreatedAt: 108,
		},
		{
			OrganizationId: organization.Id, ActorUserId: member.Id, Action: "organization.wallet.settle",
			TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "usage-wallet-internal",
			Metadata: `{"marker":"wallet-internal"}`, CreatedAt: 109,
		},
	}
	for i := range events {
		createOrganizationUsageLogEvent(t, events[i])
	}

	for _, platformUserID := range []int{platformAdmin.Id, platformRoot.Id} {
		result, err := ListOrganizationUsageLogs(platformUserID, ListOrganizationUsageLogsParams{Limit: 20})
		require.NoError(t, err)
		assert.Equal(t, int64(len(events)-1), result.Total)
		require.Len(t, result.Items, len(events)-1)
	}

	platformFiltered, err := ListOrganizationUsageLogs(platformAdmin.Id, ListOrganizationUsageLogsParams{
		Limit: 20, OrganizationID: otherOrganization.Id,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), platformFiltered.Total)
	for i := range platformFiltered.Items {
		assert.Equal(t, otherOrganization.Id, platformFiltered.Items[i].OrganizationID)
		assert.Equal(t, otherOrganization.Name, platformFiltered.Items[i].OrganizationName)
	}

	for _, managerUserID := range []int{owner.Id, admin.Id} {
		result, err := ListOrganizationUsageLogs(managerUserID, ListOrganizationUsageLogsParams{Limit: 20})
		require.NoError(t, err)
		assert.Equal(t, int64(6), result.Total)
		require.Len(t, result.Items, 6)
		for i := range result.Items {
			assert.Zero(t, result.Items[i].OrganizationID)
			assert.Equal(t, organization.Name, result.Items[i].OrganizationName)
		}
	}

	memberResult, err := ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(3), memberResult.Total)
	require.Len(t, memberResult.Items, 3)
	memberActions := organizationUsageLogActions(memberResult.Items)
	require.Contains(t, memberActions, "organization.member.actor")
	require.Contains(t, memberActions, "organization.member.initiator")
	require.Contains(t, memberActions, "organization.member.target")
	assert.NotContains(t, memberActions, "organization.unrelated")
	assert.NotContains(t, memberActions, "organization.target.type.boundary")
	assert.NotContains(t, memberActions, "organization.cross.actor")
	assert.NotContains(t, memberActions, "organization.wallet.settle")

	targetView := memberActions["organization.member.target"]
	assert.Equal(t, admin.Username, targetView.ActorUsername)
	assert.Equal(t, member.Username, targetView.TargetName)
	assert.JSONEq(t, `{}`, string(targetView.Metadata))
	assert.NotContains(t, string(targetView.Metadata), "marker")
	initiatorView := memberActions["organization.member.initiator"]
	assert.Equal(t, member.Username, initiatorView.InitiatorUsername)

	memberActorFilter, err := ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{
		Limit: 20, ActorUserID: admin.Id,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), memberActorFilter.Total)
	require.Len(t, memberActorFilter.Items, 1)
	assert.Equal(t, "organization.member.target", memberActorFilter.Items[0].Action)

	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{OrganizationID: otherOrganization.Id})
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{OrganizationID: otherOrganization.Id})
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	walletOnly, err := ListOrganizationUsageLogs(platformAdmin.Id, ListOrganizationUsageLogsParams{
		Limit: 20, Action: "organization.wallet.settle",
	})
	require.NoError(t, err)
	assert.Zero(t, walletOnly.Total)
	assert.Empty(t, walletOnly.Items)
}

func TestOrganizationUsageLogsRecheckRolesAndValidateFilters(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	createOrganizationUsageLogEvent(t, model.OrganizationAuditEvent{
		OrganizationId: organization.Id, ActorUserId: admin.Id, Action: "organization.role.recheck",
		TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "usage-role-recheck", Metadata: `{}`,
	})
	createOrganizationUsageLogEvent(t, model.OrganizationAuditEvent{
		OrganizationId: organization.Id, ActorUserId: owner.Id, Action: "organization.owner.only",
		TargetType: "organization", TargetId: strconv.Itoa(organization.Id), RequestId: "usage-owner-only", Metadata: `{}`,
	})

	managerResult, err := ListOrganizationUsageLogs(admin.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), managerResult.Total)

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", admin.Id).Update("organization_role", model.OrganizationRoleMember).Error)
	memberResult, err := ListOrganizationUsageLogs(admin.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), memberResult.Total)
	require.Len(t, memberResult.Items, 1)
	assert.Equal(t, "organization.role.recheck", memberResult.Items[0].Action)

	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Action: strings.Repeat("x", 65)})
	assert.ErrorIs(t, err, ErrTenantOrganizationRequestInvalid)
	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{OrganizationID: -1})
	assert.ErrorIs(t, err, ErrTenantOrganizationRequestInvalid)
	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{StartTimestamp: -1})
	assert.ErrorIs(t, err, ErrTenantOrganizationRequestInvalid)
	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{StartTimestamp: 20, EndTimestamp: 10})
	assert.ErrorIs(t, err, ErrTenantOrganizationRequestInvalid)

	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", organization.Id).Update("status", model.OrganizationStatusDisabled).Error)
	_, err = ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{})
	assert.ErrorIs(t, err, ErrOrganizationInactive)
	_, err = ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{})
	assert.ErrorIs(t, err, ErrOrganizationInactive)

	platformAdmin := createOrganizationManagementUser(t, db, "usage-disabled-org-platform-admin", common.RoleAdminUser, 0, "")
	platformResult, err := ListOrganizationUsageLogs(platformAdmin.Id, ListOrganizationUsageLogsParams{OrganizationID: organization.Id})
	require.NoError(t, err)
	assert.Equal(t, int64(2), platformResult.Total)

	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", organization.Id).Update("status", model.OrganizationStatusActive).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Update("organization_status", model.OrganizationMemberStatusDisabled).Error)
	_, err = ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{})
	assert.True(t, errors.Is(err, ErrOrganizationIdentityInvalid))
}

func TestOrganizationUsageLogsApplyFiltersInsideAuthorizedScope(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	otherOwner := createOrganizationManagementUser(t, db, "usage-filter-other-owner", common.RoleCommonUser, 0, "")
	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)

	for _, event := range []model.OrganizationAuditEvent{
		{OrganizationId: organization.Id, ActorUserId: admin.Id, Action: "organization.filter", TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "filter-a", Metadata: `{}`, CreatedAt: 301},
		{OrganizationId: organization.Id, ActorUserId: owner.Id, Action: "organization.other", TargetType: "organization", TargetId: strconv.Itoa(organization.Id), RequestId: "filter-b", Metadata: `{}`, CreatedAt: 302},
		{OrganizationId: otherOrganization.Id, ActorUserId: admin.Id, Action: "organization.filter", TargetType: "user", TargetId: strconv.Itoa(member.Id), RequestId: "filter-cross", Metadata: `{}`, CreatedAt: 303},
	} {
		createOrganizationUsageLogEvent(t, event)
	}

	result, err := ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{
		Limit: 1, Action: " organization.filter ", TargetType: "user", TargetID: strconv.Itoa(member.Id),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	require.Len(t, result.Items, 1)
	assert.Zero(t, result.Items[0].OrganizationID)
	assert.Equal(t, "filter-a", result.Items[0].RequestID)

	requestFiltered, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{
		Limit: 20, RequestID: "filter-b",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), requestFiltered.Total)
	require.Len(t, requestFiltered.Items, 1)
	assert.Equal(t, "organization.other", requestFiltered.Items[0].Action)

	timeFiltered, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{
		Limit: 20, StartTimestamp: 302, EndTimestamp: 302,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), timeFiltered.Total)
	require.Len(t, timeFiltered.Items, 1)
	assert.Equal(t, "filter-b", timeFiltered.Items[0].RequestID)
}

func TestOrganizationUsageLogsApplyServerSideIDVisibility(t *testing.T) {
	originalIDVisibility := custom_setting.IsIDVisibilityEnabled()
	custom_setting.SetIDVisibilityEnabled(false)
	t.Cleanup(func() {
		custom_setting.SetIDVisibilityEnabled(originalIDVisibility)
	})

	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	initiatorID := owner.Id
	metadata, err := common.Marshal(map[string]interface{}{
		"initiator_user_id":      owner.Id,
		"previous_owner_user_id": owner.Id,
		"from_organization_id":   organization.Id,
		"source_id":              strconv.Itoa(member.Id),
		"idempotency_key":        fmt.Sprintf("organization:%d:member:%d", organization.Id, member.Id),
		"safe_detail":            "kept",
		"nested": map[string]interface{}{
			"user_id": member.Id,
			"safe":    "nested-kept",
		},
	})
	require.NoError(t, err)
	createOrganizationUsageLogEvent(t, model.OrganizationAuditEvent{
		OrganizationId:  organization.Id,
		ActorUserId:     admin.Id,
		InitiatorUserId: &initiatorID,
		Action:          "organization.quota.allocate",
		TargetType:      "user",
		TargetId:        strconv.Itoa(member.Id),
		RequestId:       "usage-id-visibility",
		Metadata:        string(metadata),
	})

	tenantResult, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, tenantResult.Items, 1)
	tenantItem := tenantResult.Items[0]
	assert.Zero(t, tenantItem.OrganizationID)
	assert.Zero(t, tenantItem.ActorUserID)
	assert.Nil(t, tenantItem.InitiatorUserID)
	assert.Empty(t, tenantItem.TargetID)
	assert.Equal(t, organization.Name, tenantItem.OrganizationName)
	assert.Equal(t, admin.Username, tenantItem.ActorUsername)
	assert.Equal(t, owner.Username, tenantItem.InitiatorUsername)
	assert.Equal(t, member.Username, tenantItem.TargetName)
	encoded, err := common.Marshal(tenantItem)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"organization_id"`)
	assert.NotContains(t, string(encoded), `"actor_user_id"`)
	assert.NotContains(t, string(encoded), `"initiator_user_id"`)
	assert.NotContains(t, string(encoded), `"target_id"`)
	assert.JSONEq(t, `{
		"safe_detail": "kept",
		"nested": {"safe": "nested-kept"}
	}`, string(tenantItem.Metadata))

	platformAdmin := createOrganizationManagementUser(t, db, "usage-id-platform-admin", common.RoleAdminUser, 0, "")
	platformResult, err := ListOrganizationUsageLogs(platformAdmin.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, platformResult.Items, 1)
	assert.Equal(t, organization.Id, platformResult.Items[0].OrganizationID)
	assert.Equal(t, admin.Id, platformResult.Items[0].ActorUserID)
	require.NotNil(t, platformResult.Items[0].InitiatorUserID)
	assert.Equal(t, owner.Id, *platformResult.Items[0].InitiatorUserID)
	assert.Equal(t, strconv.Itoa(member.Id), platformResult.Items[0].TargetID)
	assert.Contains(t, string(platformResult.Items[0].Metadata), "initiator_user_id")
	assert.Contains(t, string(platformResult.Items[0].Metadata), "previous_owner_user_id")
	assert.Contains(t, string(platformResult.Items[0].Metadata), "from_organization_id")
	assert.Contains(t, string(platformResult.Items[0].Metadata), "idempotency_key")

	custom_setting.SetIDVisibilityEnabled(true)
	visibleTenantResult, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, visibleTenantResult.Items, 1)
	assert.Equal(t, organization.Id, visibleTenantResult.Items[0].OrganizationID)
	assert.Equal(t, admin.Id, visibleTenantResult.Items[0].ActorUserID)
	require.NotNil(t, visibleTenantResult.Items[0].InitiatorUserID)
	assert.Equal(t, owner.Id, *visibleTenantResult.Items[0].InitiatorUserID)
	assert.Equal(t, strconv.Itoa(member.Id), visibleTenantResult.Items[0].TargetID)
	assert.Contains(t, string(visibleTenantResult.Items[0].Metadata), "initiator_user_id")
	assert.Contains(t, string(visibleTenantResult.Items[0].Metadata), "previous_owner_user_id")
	assert.Contains(t, string(visibleTenantResult.Items[0].Metadata), "from_organization_id")
	assert.Contains(t, string(visibleTenantResult.Items[0].Metadata), "idempotency_key")
}

func TestOrganizationUsageLogsHideDefaultOrganizationFromNonPlatformRoles(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	defaultKey := model.DefaultOrganizationSystemKey
	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", organization.Id).Update("system_key", defaultKey).Error)
	createOrganizationUsageLogEvent(t, model.OrganizationAuditEvent{
		OrganizationId: organization.Id,
		ActorUserId:    member.Id,
		Action:         "organization.member.join",
		TargetType:     "user",
		TargetId:       strconv.Itoa(member.Id),
		RequestId:      "default-member-log",
		Metadata:       `{"organization_role":"member"}`,
	})

	for _, userID := range []int{owner.Id, admin.Id, member.Id} {
		_, err := ListOrganizationUsageLogs(userID, ListOrganizationUsageLogsParams{Limit: 20})
		assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	}
}

func TestOrganizationUsageLogsTreatEmptyHistoricalMetadataAsObject(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, _, _ := createTenantManagementFixture(t)
	require.NoError(t, db.Create(&model.OrganizationAuditEvent{
		OrganizationId: organization.Id,
		ActorUserId:    owner.Id,
		Action:         "organization.status.update",
		TargetType:     "organization",
		TargetId:       strconv.Itoa(organization.Id),
		RequestId:      "empty-historical-metadata",
		Metadata:       "",
	}).Error)

	result, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.JSONEq(t, `{}`, string(result.Items[0].Metadata))
}

func TestOrganizationUsageLogsTreatInvalidHistoricalMetadataAsObject(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, _, _ := createTenantManagementFixture(t)
	require.NoError(t, db.Create(&model.OrganizationAuditEvent{
		OrganizationId: organization.Id,
		ActorUserId:    owner.Id,
		Action:         "organization.status.update",
		TargetType:     "organization",
		TargetId:       strconv.Itoa(organization.Id),
		RequestId:      "invalid-historical-metadata",
		Metadata:       "not-json",
	}).Error)

	result, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.JSONEq(t, `{}`, string(result.Items[0].Metadata))
}

func TestOrganizationUsageLogsSanitizeMemberAccountingMetadata(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, _, member := createTenantManagementFixture(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	require.NoError(t, db.Model(&model.OrganizationFundAccount{}).
		Where("organization_id = ?", organization.Id).
		Update("quota", 1_000).Error)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id,
		UserId:         member.Id,
	}).Error)

	_, err := model.AllocateOrganizationQuota(model.OrganizationQuotaTransferParams{
		OrganizationId: organization.Id,
		UserId:         member.Id,
		Amount:         125,
		SourceType:     "organization_pool",
		SourceId:       "usage-log-allocation-source",
		IdempotencyKey: "usage-log-allocation-idempotency",
		RequestId:      "usage-log-allocation-request",
		Actor: model.OrganizationAccountingActor{
			Kind:   model.OrganizationAccountingActorUser,
			UserId: owner.Id,
			Policy: "organization_owner",
		},
	})
	require.NoError(t, err)
	_, err = model.CreditOrganizationFund(model.OrganizationFundCreditParams{
		OrganizationId: organization.Id,
		Amount:         50,
		SourceType:     "member_topup",
		SourceId:       "usage-log-topup-source",
		IdempotencyKey: "usage-log-topup-idempotency",
		RequestId:      "usage-log-topup-request",
		Actor: model.OrganizationAccountingActor{
			Kind:            model.OrganizationAccountingActorSystem,
			InitiatorUserId: member.Id,
			Policy:          "payment_settlement",
		},
	})
	require.NoError(t, err)

	memberResult, err := ListOrganizationUsageLogs(member.Id, ListOrganizationUsageLogsParams{Limit: 20})
	require.NoError(t, err)
	memberLogs := organizationUsageLogActions(memberResult.Items)
	require.Contains(t, memberLogs, "organization.quota.allocate")
	require.Contains(t, memberLogs, "organization.fund.credit")
	assert.JSONEq(t, `{
		"user_quota_delta": 125,
		"user_quota_after": 125,
		"recoverable_quota_delta": 125,
		"recoverable_quota_after": 125
	}`, string(memberLogs["organization.quota.allocate"].Metadata))
	assert.JSONEq(t, `{"amount": 50}`, string(memberLogs["organization.fund.credit"].Metadata))
	for _, item := range memberResult.Items {
		metadata := string(item.Metadata)
		assert.NotContains(t, metadata, "pool_quota_after")
		assert.NotContains(t, metadata, "pool_quota_delta")
		assert.NotContains(t, metadata, "idempotency_key")
		assert.NotContains(t, metadata, "source_id")
		assert.NotContains(t, metadata, "actor_policy")
	}

	ownerResult, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{
		Limit:  20,
		Action: "organization.quota.allocate",
	})
	require.NoError(t, err)
	require.Len(t, ownerResult.Items, 1)
	assert.Contains(t, string(ownerResult.Items[0].Metadata), "pool_quota_after")
	assert.NotContains(t, string(ownerResult.Items[0].Metadata), "idempotency_key")
}

func TestOrganizationUsageLogsReportTotalBeyondCurrentPage(t *testing.T) {
	setupOrganizationManagementTestDB(t)
	organization, owner, _, member := createTenantManagementFixture(t)
	for index := 0; index < 3; index++ {
		createOrganizationUsageLogEvent(t, model.OrganizationAuditEvent{
			OrganizationId: organization.Id,
			ActorUserId:    owner.Id,
			Action:         "organization.pagination",
			TargetType:     "user",
			TargetId:       strconv.Itoa(member.Id),
			RequestId:      fmt.Sprintf("usage-pagination-%d", index),
			Metadata:       `{}`,
			CreatedAt:      int64(index + 1),
		})
	}

	result, err := ListOrganizationUsageLogs(owner.Id, ListOrganizationUsageLogsParams{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Total)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "usage-pagination-2", result.Items[0].RequestID)
}
