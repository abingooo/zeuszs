package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateOrganizationWithOwnerForPlatformCommitsTenantAccountAndLedgerTogether(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	actor := createOrganizationManagementUser(t, db, "owner-provision-admin", common.RoleAdminUser, 0, "")
	previousInitialQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 321
	t.Cleanup(func() { common.QuotaForNewUser = previousInitialQuota })

	result, err := CreateOrganizationWithOwnerForPlatform(actor.Id, CreateOrganizationWithOwnerParams{
		Name:             "  Applied AI Lab  ",
		OwnerUsername:    "lab-owner",
		OwnerPassword:    "password123",
		OwnerDisplayName: "Lab Owner",
		OwnerEmail:       " OWNER@EXAMPLE.COM ",
		AllowMemberTopup: common.GetPointer(false),
		RequestID:        "owner-provision-create-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Applied AI Lab", result.Organization.Name)
	assert.False(t, result.Organization.AllowMemberTopup)
	assert.Equal(t, result.Organization.Id, result.Owner.OrganizationID)
	assert.Equal(t, model.OrganizationRoleOwner, result.Owner.OrganizationRole)
	assert.Equal(t, "owner@example.com", result.Owner.Email)

	var owner model.User
	require.NoError(t, db.Where("id = ?", result.Owner.UserID).First(&owner).Error)
	assert.Equal(t, common.RoleCommonUser, owner.Role)
	assert.Equal(t, result.Organization.Id, owner.OrganizationId)
	assert.Equal(t, model.OrganizationRoleOwner, owner.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, owner.OrganizationStatus)
	assert.Equal(t, 321, owner.Quota)
	assert.NotEqual(t, "password123", owner.Password)

	var ledger model.OrganizationQuotaLedger
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", result.Organization.Id, owner.Id).First(&ledger).Error)
	assert.Equal(t, model.OrganizationLedgerWalletCredit, ledger.Operation)
	assert.Equal(t, int64(321), ledger.UserQuotaDelta)

	var audit model.OrganizationAuditEvent
	require.NoError(t, db.Where("organization_id = ? AND action = ?", result.Organization.Id, "organization.create").First(&audit).Error)
	assert.Equal(t, "organization.create", audit.Action)
}

func TestCreateOrganizationWithOwnerForPlatformRejectsTenantRoleAndRollsBackDuplicateOwner(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	tenantOwner := createOrganizationManagementUser(t, db, "tenant-only-owner", common.RoleCommonUser, 0, "")
	admin := createOrganizationManagementUser(t, db, "owner-provision-root", common.RoleRootUser, 0, "")
	createOrganizationManagementUser(t, db, "duplicate-owner", common.RoleCommonUser, 0, "")

	_, err := CreateOrganizationWithOwnerForPlatform(tenantOwner.Id, CreateOrganizationWithOwnerParams{
		Name:          "Forbidden Lab",
		OwnerUsername: "forbidden-owner",
		OwnerPassword: "password123",
	})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)

	var organizationsBefore int64
	require.NoError(t, db.Model(&model.Organization{}).Count(&organizationsBefore).Error)
	_, err = CreateOrganizationWithOwnerForPlatform(admin.Id, CreateOrganizationWithOwnerParams{
		Name:          "Duplicate Lab",
		OwnerUsername: "duplicate-owner",
		OwnerPassword: "password123",
	})
	require.Error(t, err)
	var organizationsAfter int64
	require.NoError(t, db.Model(&model.Organization{}).Count(&organizationsAfter).Error)
	assert.Equal(t, organizationsBefore, organizationsAfter)
}

func TestCreditOrganizationFundForPlatformIsIdempotentAndPlatformOnly(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.OrganizationQuotaLedger{}, &model.OrganizationQuotaOperation{}))
	admin := createOrganizationManagementUser(t, db, "fund-credit-admin", common.RoleAdminUser, 0, "")
	tenantOwner := createOrganizationManagementUser(t, db, "fund-credit-owner", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, tenantOwner.Id, model.OrganizationStatusActive)

	params := CreditOrganizationFundForPlatformParams{
		OrganizationID: organization.Id,
		Amount:         1200,
		Reference:      "offline-receipt-42",
		RequestID:      "fund-credit-request-42",
	}
	first, err := CreditOrganizationFundForPlatform(admin.Id, params)
	require.NoError(t, err)
	assert.False(t, first.AlreadyApplied)
	assert.Equal(t, int64(1200), first.PoolQuotaAfter)
	retryParams := params
	retryParams.RequestID = "fund-credit-request-42-retry"
	second, err := CreditOrganizationFundForPlatform(admin.Id, retryParams)
	require.NoError(t, err)
	assert.True(t, second.AlreadyApplied)
	assert.Equal(t, first.LedgerId, second.LedgerId)

	conflictingParams := retryParams
	conflictingParams.Amount = 1201
	conflictingParams.RequestID = "fund-credit-request-42-conflict"
	_, err = CreditOrganizationFundForPlatform(admin.Id, conflictingParams)
	assert.ErrorIs(t, err, model.ErrOrganizationAccountingIdempotency)

	var account model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&account).Error)
	assert.Equal(t, int64(1200), account.Quota)

	_, err = CreditOrganizationFundForPlatform(admin.Id, CreditOrganizationFundForPlatformParams{
		OrganizationID: organization.Id,
		Amount:         1,
		RequestID:      "fund-credit-missing-reference",
	})
	assert.ErrorIs(t, err, ErrOrganizationFundReferenceRequired)

	_, err = CreditOrganizationFundForPlatform(tenantOwner.Id, CreditOrganizationFundForPlatformParams{
		OrganizationID: organization.Id,
		Amount:         1,
		Reference:      "offline-receipt-forbidden",
		RequestID:      "fund-credit-forbidden",
	})
	assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
}
