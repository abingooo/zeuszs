package service

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func tenantPrincipal(user model.User) OrganizationPrincipal {
	return OrganizationPrincipal{
		UserID: user.Id, OrganizationID: user.OrganizationId,
		Role: user.OrganizationRole, PlatformRole: user.Role,
	}
}

func createTenantManagementFixture(t *testing.T) (model.Organization, model.User, model.User, model.User) {
	t.Helper()
	db := model.DB
	owner := createOrganizationManagementUser(t, db, "tenant-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	owner.OrganizationId = organization.Id
	owner.OrganizationRole = model.OrganizationRoleOwner
	owner.OrganizationStatus = model.OrganizationMemberStatusActive
	admin := createOrganizationManagementUser(t, db, "tenant-admin", common.RoleCommonUser, organization.Id, model.OrganizationRoleAdmin)
	member := createOrganizationManagementUser(t, db, "tenant-member", common.RoleCommonUser, organization.Id, model.OrganizationRoleMember)
	return organization, owner, admin, member
}

func TestTenantOrganizationSummaryAndMemberListStayInScope(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	require.NoError(t, db.Model(&model.OrganizationFundAccount{}).Where("organization_id = ?", organization.Id).Update("quota", 900).Error)
	secondMember := createOrganizationManagementUser(t, db, "tenant-member-disabled", common.RoleAdminUser, organization.Id, model.OrganizationRoleMember)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", secondMember.Id).Update("organization_status", model.OrganizationMemberStatusDisabled).Error)
	limit := int64(700)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{
		OrganizationId: organization.Id, UserId: member.Id, RecoverableQuota: 300,
		ConsumedQuota: 25, ConsumptionLimit: &limit,
	}).Error)

	otherOwner := createOrganizationManagementUser(t, db, "other-owner", common.RoleCommonUser, 0, "")
	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id": otherOrganization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	_ = createOrganizationManagementUser(t, db, "other-member", common.RoleCommonUser, otherOrganization.Id, model.OrganizationRoleMember)

	memberSummary, err := GetTenantOrganizationSummary(tenantPrincipal(member))
	require.NoError(t, err)
	assert.Equal(t, organization.Id, memberSummary.OrganizationID)
	assert.False(t, memberSummary.IsDefault)
	assert.Equal(t, model.OrganizationRoleMember, memberSummary.CurrentUserRole)
	assert.Nil(t, memberSummary.OwnerUserID)
	assert.Nil(t, memberSummary.PolicyVersion)
	assert.Nil(t, memberSummary.FundQuota)
	assert.Nil(t, memberSummary.MemberCount)
	assert.Nil(t, memberSummary.CreatedAt)
	assert.Nil(t, memberSummary.UpdatedAt)
	encodedMemberSummary, err := common.Marshal(memberSummary)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedMemberSummary), `"fund_quota"`)
	assert.NotContains(t, string(encodedMemberSummary), `"member_count"`)

	managementSummary, err := GetTenantOrganizationSummary(tenantPrincipal(admin))
	require.NoError(t, err)
	require.NotNil(t, managementSummary.FundQuota)
	require.NotNil(t, managementSummary.MemberCount)
	assert.EqualValues(t, 900, *managementSummary.FundQuota)
	assert.EqualValues(t, 2, *managementSummary.MemberCount)
	assert.False(t, managementSummary.IsDefault)

	result, err := ListTenantOrganizationMembers(tenantPrincipal(owner), ListOrganizationMembersParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	for _, item := range result.Items {
		assert.Equal(t, organization.Id, item.OrganizationID)
		assert.Equal(t, model.OrganizationRoleMember, item.OrganizationRole)
		assert.NotEqual(t, owner.Id, item.UserID)
		assert.NotEqual(t, admin.Id, item.UserID)
	}
	assert.Equal(t, int64(300), result.Items[0].RecoverableQuota)
	assert.Equal(t, &limit, result.Items[0].ConsumptionLimit)

	_, err = ListTenantOrganizationMembers(tenantPrincipal(member), ListOrganizationMembersParams{Limit: 10})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
}

func TestDefaultOrganizationSummaryRejectsOrdinaryMember(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	owner := createOrganizationManagementUser(t, db, "default-tenant-owner", common.RoleCommonUser, 0, "")
	systemKey := model.DefaultOrganizationSystemKey
	organization := model.Organization{
		Name: "Renamed platform tenant", SystemKey: &systemKey,
		Status: model.OrganizationStatusActive, OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	owner.OrganizationId = organization.Id
	owner.OrganizationRole = model.OrganizationRoleOwner
	owner.OrganizationStatus = model.OrganizationMemberStatusActive
	member := createOrganizationManagementUser(t, db, "default-tenant-member", common.RoleCommonUser, organization.Id, model.OrganizationRoleMember)

	_, err := GetTenantOrganizationSummary(tenantPrincipal(member))
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)

	summary, err := GetTenantOrganizationSummary(tenantPrincipal(owner))
	require.NoError(t, err)
	assert.True(t, summary.IsDefault)
	assert.Equal(t, "Renamed platform tenant", summary.Name)
}

func TestTenantInviteAndTopupPolicyRequireOrganizationAdminRole(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	platformAdminMember := createOrganizationManagementUser(t, db, "platform-admin-member", common.RoleAdminUser, organization.Id, model.OrganizationRoleMember)

	created, err := CreateTenantOrganizationInvite(tenantPrincipal(owner), CreateTenantOrganizationInviteParams{
		Code: "TENANT-RESEARCH", MaxUses: 3, RequestID: "tenant-invite-create",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationRoleMember, created.DefaultRole)
	assert.Equal(t, "TENANT-RESEARCH", created.Code)

	listed, err := ListTenantOrganizationInvites(tenantPrincipal(admin), ListOrganizationInvitesParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Empty(t, listed.Items[0].Code)
	assert.Equal(t, organization.Id, listed.Items[0].OrganizationID)

	disabled, err := DisableTenantOrganizationInvite(tenantPrincipal(admin), DisableTenantOrganizationInviteParams{
		InviteID: created.ID, RequestID: "tenant-invite-disable",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationInviteStatusDisabled, disabled.Status)

	_, err = CreateTenantOrganizationInvite(tenantPrincipal(platformAdminMember), CreateTenantOrganizationInviteParams{Code: "PLATFORM-CANNOT-BYPASS"})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	_, err = UpdateTenantOrganizationTopupPolicy(tenantPrincipal(member), UpdateTenantOrganizationTopupPolicyParams{AllowMemberTopup: false})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)

	updated, err := UpdateTenantOrganizationTopupPolicy(tenantPrincipal(admin), UpdateTenantOrganizationTopupPolicyParams{
		AllowMemberTopup: false, RequestID: "tenant-topup-policy",
	})
	require.NoError(t, err)
	assert.False(t, updated.AllowMemberTopup)
	assert.EqualValues(t, 2, updated.PolicyVersion)

	for _, actor := range []model.User{owner, admin} {
		_, err := AssignOrganizationMemberRoleForPlatform(actor.Id, AssignOrganizationRoleParams{
			OrganizationID: organization.Id, UserID: member.Id, Role: model.OrganizationRoleAdmin,
		})
		assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
	}
}

func TestTenantMemberOperationsCannotMutateOwnerOrAdmin(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	organization, owner, admin, member := createTenantManagementFixture(t)
	require.NoError(t, db.Create(&model.OrganizationMemberFund{OrganizationId: organization.Id, UserId: member.Id}).Error)
	for index := 1; index <= 2; index++ {
		token := model.Token{
			UserId: member.Id, Key: fmt.Sprintf("tenant-member-token-%d", index), Name: "member token",
			Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		}
		require.NoError(t, db.Create(&token).Error)
	}
	adminToken := model.Token{UserId: admin.Id, Key: "tenant-admin-token", Name: "admin token", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&adminToken).Error)

	limit := int64(1000)
	view, err := UpdateTenantOrganizationMemberLimit(tenantPrincipal(owner), UpdateTenantOrganizationMemberLimitParams{
		UserID: member.Id, ConsumptionLimit: &limit, RequestID: "tenant-limit",
	})
	require.NoError(t, err)
	assert.Equal(t, &limit, view.ConsumptionLimit)

	view, err = UpdateTenantOrganizationMemberStatus(tenantPrincipal(admin), UpdateTenantOrganizationMemberStatusParams{
		UserID: member.Id, Status: model.OrganizationMemberStatusDisabled, RequestID: "tenant-disable-member",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationMemberStatusDisabled, view.OrganizationStatus)
	view, err = UpdateTenantOrganizationMemberStatus(tenantPrincipal(owner), UpdateTenantOrganizationMemberStatusParams{
		UserID: member.Id, Status: model.OrganizationMemberStatusActive, RequestID: "tenant-enable-member",
	})
	require.NoError(t, err)
	assert.Equal(t, model.OrganizationMemberStatusActive, view.OrganizationStatus)

	_, err = UpdateTenantOrganizationMemberStatus(tenantPrincipal(owner), UpdateTenantOrganizationMemberStatusParams{
		UserID: admin.Id, Status: model.OrganizationMemberStatusDisabled,
	})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	_, err = UpdateTenantOrganizationMemberLimit(tenantPrincipal(owner), UpdateTenantOrganizationMemberLimitParams{
		UserID: admin.Id, ConsumptionLimit: &limit,
	})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)

	disabled, err := DisableTenantOrganizationMemberTokens(tenantPrincipal(owner), DisableTenantOrganizationMemberTokensParams{
		UserID: member.Id, RequestID: "tenant-disable-tokens",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, disabled)
	var enabledMemberTokens int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ? AND status = ?", member.Id, common.TokenStatusEnabled).Count(&enabledMemberTokens).Error)
	assert.Zero(t, enabledMemberTokens)
	var persistedAdminToken model.Token
	require.NoError(t, db.First(&persistedAdminToken, adminToken.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, persistedAdminToken.Status)

	_, err = DisableTenantOrganizationMemberTokens(tenantPrincipal(owner), DisableTenantOrganizationMemberTokensParams{UserID: admin.Id})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
}

func TestTenantQuotaAllocationAndRecoveryAreAuthorizedAndIdempotent(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaOperation{}, &model.OrganizationQuotaLedger{}))
	organization, owner, admin, member := createTenantManagementFixture(t)
	require.NoError(t, db.Model(&model.OrganizationFundAccount{}).
		Where("organization_id = ?", organization.Id).Update("quota", 100).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Update("quota", 20).Error)

	allocateParams := TransferTenantOrganizationQuotaParams{
		UserID: member.Id, Amount: 60, RequestID: "tenant-allocate-request", IdempotencyKey: "tenant-allocate-idempotency",
	}
	allocated, err := AllocateTenantOrganizationQuota(tenantPrincipal(owner), allocateParams)
	require.NoError(t, err)
	assert.False(t, allocated.AlreadyApplied)
	assert.EqualValues(t, 80, allocated.UserQuotaAfter)
	assert.EqualValues(t, 40, allocated.PoolQuotaAfter)
	assert.EqualValues(t, 60, allocated.RecoverableQuotaAfter)

	replayed, err := AllocateTenantOrganizationQuota(tenantPrincipal(owner), allocateParams)
	require.NoError(t, err)
	assert.True(t, replayed.AlreadyApplied)
	assert.Equal(t, allocated.LedgerID, replayed.LedgerID)

	_, err = AllocateTenantOrganizationQuota(tenantPrincipal(admin), TransferTenantOrganizationQuotaParams{
		UserID: member.Id, Amount: 50, RequestID: "tenant-allocate-insufficient", IdempotencyKey: "tenant-allocate-insufficient-idempotency",
	})
	assert.ErrorIs(t, err, model.ErrOrganizationFundInsufficient)
	_, err = RecoverTenantOrganizationQuota(tenantPrincipal(admin), TransferTenantOrganizationQuotaParams{
		UserID: member.Id, Amount: 70, RequestID: "tenant-recover-too-much", IdempotencyKey: "tenant-recover-too-much-idempotency",
	})
	assert.ErrorIs(t, err, model.ErrOrganizationRecoverableInsufficient)

	recovered, err := RecoverTenantOrganizationQuota(tenantPrincipal(admin), TransferTenantOrganizationQuotaParams{
		UserID: member.Id, Amount: 20, RequestID: "tenant-recover-request", IdempotencyKey: "tenant-recover-idempotency",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 60, recovered.UserQuotaAfter)
	assert.EqualValues(t, 60, recovered.PoolQuotaAfter)
	assert.EqualValues(t, 40, recovered.RecoverableQuotaAfter)

	_, err = AllocateTenantOrganizationQuota(tenantPrincipal(member), TransferTenantOrganizationQuotaParams{
		UserID: member.Id, Amount: 1, RequestID: "member-cannot-allocate", IdempotencyKey: "member-cannot-allocate-idempotency",
	})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	_, err = AllocateTenantOrganizationQuota(tenantPrincipal(owner), TransferTenantOrganizationQuotaParams{
		UserID: admin.Id, Amount: 1, RequestID: "admin-cannot-be-target", IdempotencyKey: "admin-cannot-be-target-idempotency",
	})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)

	var persistedUser model.User
	require.NoError(t, db.First(&persistedUser, member.Id).Error)
	assert.Equal(t, 60, persistedUser.Quota)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.OrganizationQuotaLedger{}).Where("organization_id = ?", organization.Id).Count(&ledgerCount).Error)
	assert.EqualValues(t, 2, ledgerCount)
	var operation model.OrganizationQuotaOperation
	require.NoError(t, db.Where("ledger_id = ?", allocated.LedgerID).First(&operation).Error)
	assert.NotContains(t, operation.IdempotencyKey, allocateParams.IdempotencyKey)
	_, otherOrganizationKey, err := normalizeTenantOrganizationAccountingKeys(
		organization.Id+1, "allocate", member.Id, allocateParams.RequestID, allocateParams.IdempotencyKey,
	)
	require.NoError(t, err)
	assert.NotEqual(t, operation.IdempotencyKey, otherOrganizationKey)
}

func TestTenantLedgerAndAuditQueriesNeverCrossOrganizations(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}))
	organization, owner, _, member := createTenantManagementFixture(t)
	otherOwner := createOrganizationManagementUser(t, db, "ledger-other-owner", common.RoleCommonUser, 0, "")
	otherOrganization := createOrganizationManagementOrganization(t, db, otherOwner.Id, model.OrganizationStatusActive)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherOwner.Id).Updates(map[string]interface{}{
		"organization_id": otherOrganization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	otherOwner.OrganizationId = otherOrganization.Id
	otherOwner.OrganizationRole = model.OrganizationRoleOwner

	for index, organizationID := range []int{organization.Id, otherOrganization.Id} {
		require.NoError(t, db.Create(&model.OrganizationQuotaLedger{
			OrganizationId: organizationID, UserId: member.Id, Operation: model.OrganizationLedgerAllocate,
			SourceType: "test", SourceId: strconv.Itoa(index + 1), ActorUserId: owner.Id,
			IdempotencyKey: fmt.Sprintf("tenant-ledger-%d", index), Fingerprint: fmt.Sprintf("fingerprint-%d", index),
			RequestId: fmt.Sprintf("tenant-ledger-request-%d", index), Status: model.OrganizationLedgerStatusCommitted,
		}).Error)
		require.NoError(t, db.Create(&model.OrganizationAuditEvent{
			OrganizationId: organizationID, ActorUserId: owner.Id, Action: "organization.test",
			TargetType: "organization", TargetId: strconv.Itoa(organizationID),
			RequestId: fmt.Sprintf("tenant-audit-%d", index), Metadata: fmt.Sprintf(`{"organization_id":%d}`, organizationID),
		}).Error)
	}

	ledger, err := ListTenantOrganizationLedger(tenantPrincipal(owner), ListTenantOrganizationLedgerParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, ledger.Items, 1)
	assert.Equal(t, "1", ledger.Items[0].SourceID)

	audit, err := ListTenantOrganizationAudit(tenantPrincipal(owner), ListTenantOrganizationAuditParams{Limit: 10, Action: "organization.test"})
	require.NoError(t, err)
	require.Len(t, audit.Items, 1)
	var metadata map[string]interface{}
	require.NoError(t, common.Unmarshal(audit.Items[0].Metadata, &metadata))
	assert.EqualValues(t, organization.Id, metadata["organization_id"])

	_, err = ListTenantOrganizationLedger(tenantPrincipal(member), ListTenantOrganizationLedgerParams{Limit: 10})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	_, err = ListTenantOrganizationAudit(tenantPrincipal(member), ListTenantOrganizationAuditParams{Limit: 10})
	assert.ErrorIs(t, err, ErrOrganizationActionForbidden)
	assert.False(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestTenantLedgerAndAuditViewsExposeInitiatorAndHandleLegacyNull(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}))
	organization, owner, _, member := createTenantManagementFixture(t)
	initiatorUserID := member.Id

	ledgers := []model.OrganizationQuotaLedger{
		{
			OrganizationId: organization.Id, Operation: model.OrganizationLedgerFundCredit,
			SourceType: "payment_topup", SourceId: "tenant-initiator-payment", ActorUserId: 0,
			InitiatorUserId: &initiatorUserID, IdempotencyKey: "tenant-initiator-ledger",
			Fingerprint: "tenant-initiator-fingerprint", RequestId: "tenant-initiator-request",
			Status: model.OrganizationLedgerStatusCommitted,
		},
		{
			OrganizationId: organization.Id, Operation: model.OrganizationLedgerFundCredit,
			SourceType: "legacy", SourceId: "tenant-legacy-payment", ActorUserId: 0,
			IdempotencyKey: "tenant-legacy-ledger", Fingerprint: "tenant-legacy-fingerprint",
			RequestId: "tenant-legacy-request", Status: model.OrganizationLedgerStatusCommitted,
		},
	}
	for index := range ledgers {
		require.NoError(t, db.Create(&ledgers[index]).Error)
	}

	events := []model.OrganizationAuditEvent{
		{
			OrganizationId: organization.Id, ActorUserId: 0, InitiatorUserId: &initiatorUserID,
			Action: "organization.fund.credit", TargetType: "organization", TargetId: strconv.Itoa(organization.Id),
			RequestId: "tenant-initiator-request", Metadata: `{"initiator_user_id":1}`,
		},
		{
			OrganizationId: organization.Id, ActorUserId: 0, Action: "organization.fund.credit",
			TargetType: "organization", TargetId: strconv.Itoa(organization.Id),
			RequestId: "tenant-legacy-request", Metadata: `{}`,
		},
	}
	for index := range events {
		require.NoError(t, db.Create(&events[index]).Error)
	}

	ledgerResult, err := ListTenantOrganizationLedger(tenantPrincipal(owner), ListTenantOrganizationLedgerParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, ledgerResult.Items, 2)
	ledgerBySource := make(map[string]TenantOrganizationLedgerView, len(ledgerResult.Items))
	for _, item := range ledgerResult.Items {
		ledgerBySource[item.SourceID] = item
	}
	require.NotNil(t, ledgerBySource["tenant-initiator-payment"].InitiatorUserID)
	assert.Equal(t, initiatorUserID, *ledgerBySource["tenant-initiator-payment"].InitiatorUserID)
	assert.Nil(t, ledgerBySource["tenant-legacy-payment"].InitiatorUserID)

	auditResult, err := ListTenantOrganizationAudit(tenantPrincipal(owner), ListTenantOrganizationAuditParams{
		Limit: 10, Action: "organization.fund.credit",
	})
	require.NoError(t, err)
	require.Len(t, auditResult.Items, 2)
	auditByRequest := make(map[string]TenantOrganizationAuditView, len(auditResult.Items))
	for _, item := range auditResult.Items {
		auditByRequest[item.RequestID] = item
	}
	require.NotNil(t, auditByRequest["tenant-initiator-request"].InitiatorUserID)
	assert.Equal(t, initiatorUserID, *auditByRequest["tenant-initiator-request"].InitiatorUserID)
	assert.Nil(t, auditByRequest["tenant-legacy-request"].InitiatorUserID)
}
