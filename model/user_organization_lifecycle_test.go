package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserOrganizationLifecycleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousRedis := common.RedisEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&User{}, &UserSession{}, &Token{}, &AuthFlow{}, &ExternalIdentityClaim{},
		&TwoFA{}, &TwoFABackupCode{}, &PasskeyCredential{}, &UserOAuthBinding{},
		&Organization{}, &OrganizationFundAccount{}, &OrganizationMemberFund{},
		&OrganizationWalletReservation{}, &Task{}, &Midjourney{},
	))
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createUserLifecycleOrganization(t *testing.T, db *gorm.DB, owner *User) Organization {
	t.Helper()
	organization := Organization{
		Name: "Lifecycle Organization", Status: OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	return organization
}

func TestUserUpdateCannotMutateOrganizationMembership(t *testing.T) {
	db := setupUserOrganizationLifecycleTestDB(t)
	owner := User{Username: "lifecycle-owner", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "lifecycle-owner-aff"}
	require.NoError(t, db.Create(&owner).Error)
	organization := createUserLifecycleOrganization(t, db, &owner)
	otherOwner := User{Username: "other-owner", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "other-owner-aff"}
	require.NoError(t, db.Create(&otherOwner).Error)
	otherOrganization := createUserLifecycleOrganization(t, db, &otherOwner)

	member := User{
		Username: "lifecycle-member", Password: "password", DisplayName: "before",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "lifecycle-member-aff",
		OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
		OrganizationStatus: OrganizationMemberStatusActive, AuthVersion: 1,
	}
	require.NoError(t, db.Create(&member).Error)

	forged, err := GetUserById(member.Id, true)
	require.NoError(t, err)
	forged.DisplayName = "after"
	forged.OrganizationId = otherOrganization.Id
	forged.OrganizationRole = OrganizationRoleAdmin
	forged.OrganizationStatus = OrganizationMemberStatusDisabled
	require.NoError(t, forged.Update(false))

	var persisted User
	require.NoError(t, db.First(&persisted, member.Id).Error)
	assert.Equal(t, "after", persisted.DisplayName)
	assert.Equal(t, organization.Id, persisted.OrganizationId)
	assert.Equal(t, OrganizationRoleMember, persisted.OrganizationRole)
	assert.Equal(t, OrganizationMemberStatusActive, persisted.OrganizationStatus)
}

func TestOrganizationOwnerCannotBeDisabledOrDeleted(t *testing.T) {
	db := setupUserOrganizationLifecycleTestDB(t)
	owner := User{
		Username: "protected-owner", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, AffCode: "protected-owner-aff", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&owner).Error)
	organization := createUserLifecycleOrganization(t, db, &owner)
	require.NoError(t, db.Model(&owner).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": OrganizationRoleOwner,
		"organization_status": OrganizationMemberStatusActive,
	}).Error)

	owner.Status = common.UserStatusDisabled
	assert.ErrorIs(t, owner.Update(false), ErrOrganizationOwnerDisableForbidden)
	assert.ErrorIs(t, owner.Delete(), ErrOrganizationOwnerDeletionForbidden)
	assert.ErrorIs(t, owner.HardDelete(), ErrOrganizationOwnerDeletionForbidden)

	var persisted User
	require.NoError(t, db.First(&persisted, owner.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, persisted.Status)
	assert.False(t, persisted.DeletedAt.Valid)
}

func TestOrganizationOwnerPointerFailsClosedWhenMembershipRoleIsCorrupt(t *testing.T) {
	db := setupUserOrganizationLifecycleTestDB(t)
	owner := User{
		Username: "corrupt-owner", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AffCode: "corrupt-owner-aff", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&owner).Error)
	organization := createUserLifecycleOrganization(t, db, &owner)
	require.NoError(t, db.Model(&owner).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": OrganizationRoleMember,
		"organization_status": OrganizationMemberStatusActive,
	}).Error)

	assert.ErrorIs(t, owner.Delete(), ErrOrganizationOwnerDeletionForbidden)
	var count int64
	require.NoError(t, db.Model(&User{}).Where("id = ?", owner.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestOrganizationMemberDeletionRequiresFundsRecovery(t *testing.T) {
	db := setupUserOrganizationLifecycleTestDB(t)
	owner := User{Username: "fund-owner", Password: "password", Status: common.UserStatusEnabled, AffCode: "fund-owner-aff"}
	require.NoError(t, db.Create(&owner).Error)
	organization := createUserLifecycleOrganization(t, db, &owner)
	member := User{
		Username: "fund-member", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AffCode: "fund-member-aff", AuthVersion: 1,
		OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
		OrganizationStatus: OrganizationMemberStatusActive, Quota: 50,
	}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&OrganizationMemberFund{
		OrganizationId: organization.Id, UserId: member.Id, RecoverableQuota: 30,
	}).Error)

	assert.ErrorIs(t, member.Delete(), ErrOrganizationMemberFundsOutstanding)
	require.NoError(t, db.Model(&OrganizationMemberFund{}).
		Where("organization_id = ? AND user_id = ?", organization.Id, member.Id).
		Update("recoverable_quota", 0).Error)
	require.NoError(t, member.Delete())

	var persisted User
	require.NoError(t, db.Unscoped().First(&persisted, member.Id).Error)
	assert.True(t, persisted.DeletedAt.Valid)
}

func TestOrganizationMemberHardDeleteRemovesZeroFundState(t *testing.T) {
	db := setupUserOrganizationLifecycleTestDB(t)
	owner := User{Username: "hard-owner", Password: "password", Status: common.UserStatusEnabled, AffCode: "hard-owner-aff"}
	require.NoError(t, db.Create(&owner).Error)
	organization := createUserLifecycleOrganization(t, db, &owner)
	member := User{
		Username: "hard-member", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AffCode: "hard-member-aff", AuthVersion: 1,
		OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
		OrganizationStatus: OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&OrganizationMemberFund{OrganizationId: organization.Id, UserId: member.Id}).Error)

	require.NoError(t, member.HardDelete())
	var count int64
	require.NoError(t, db.Unscoped().Model(&User{}).Where("id = ?", member.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Unscoped().Model(&OrganizationMemberFund{}).Where("user_id = ?", member.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestOrganizationMemberDeletionWaitsForRefundableAsyncBilling(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *gorm.DB, User, Organization)
	}{
		{
			name: "reserved Midjourney rejection awaiting recovery",
			seed: func(t *testing.T, db *gorm.DB, member User, organization Organization) {
				t.Helper()
				require.NoError(t, db.Create(&Midjourney{
					UserId: member.Id, OrganizationId: organization.Id, MjId: "rejected-pending-refund",
					Status: "FAILURE", Progress: "100%", Quota: 10,
					BillingStatus: MidjourneyBillingStatusReserved,
				}).Error)
			},
		},
		{
			name: "settled Midjourney still in progress",
			seed: func(t *testing.T, db *gorm.DB, member User, organization Organization) {
				t.Helper()
				require.NoError(t, db.Create(&Midjourney{
					UserId: member.Id, OrganizationId: organization.Id, MjId: "settled-in-progress",
					Status: "IN_PROGRESS", Progress: "50%", Quota: 10,
					BillingStatus: MidjourneyBillingStatusSettled,
				}).Error)
			},
		},
		{
			name: "settled Midjourney failure awaiting refund",
			seed: func(t *testing.T, db *gorm.DB, member User, organization Organization) {
				t.Helper()
				require.NoError(t, db.Create(&Midjourney{
					UserId: member.Id, OrganizationId: organization.Id, MjId: "settled-failure",
					Status: "FAILURE", Progress: "100%", Quota: 10,
					BillingStatus: MidjourneyBillingStatusSettled,
				}).Error)
			},
		},
		{
			name: "settled generic task still in progress",
			seed: func(t *testing.T, db *gorm.DB, member User, organization Organization) {
				t.Helper()
				require.NoError(t, db.Create(&Task{
					UserId: member.Id, OrganizationId: organization.Id, TaskID: "generic-in-progress",
					Status: TaskStatusInProgress, Progress: "50%", Quota: 10,
				}).Error)
			},
		},
		{
			name: "generic task failure awaiting refund",
			seed: func(t *testing.T, db *gorm.DB, member User, organization Organization) {
				t.Helper()
				require.NoError(t, db.Create(&Task{
					UserId: member.Id, OrganizationId: organization.Id, TaskID: "generic-failure",
					Status: TaskStatusFailure, Progress: "100%", Quota: 10,
				}).Error)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupUserOrganizationLifecycleTestDB(t)
			owner := User{Username: "async-owner", Password: "password", Status: common.UserStatusEnabled, AffCode: "async-owner-aff"}
			require.NoError(t, db.Create(&owner).Error)
			organization := createUserLifecycleOrganization(t, db, &owner)
			member := User{
				Username: "async-member", Password: "password", Role: common.RoleCommonUser,
				Status: common.UserStatusEnabled, AffCode: "async-member-aff", AuthVersion: 1,
				OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
				OrganizationStatus: OrganizationMemberStatusActive,
			}
			require.NoError(t, db.Create(&member).Error)
			require.NoError(t, db.Create(&OrganizationMemberFund{OrganizationId: organization.Id, UserId: member.Id}).Error)
			test.seed(t, db, member, organization)

			assert.ErrorIs(t, member.Delete(), ErrOrganizationMemberFundsOutstanding)
			assert.ErrorIs(t, member.HardDelete(), ErrOrganizationMemberFundsOutstanding)
			var persisted User
			require.NoError(t, db.First(&persisted, member.Id).Error)
			assert.False(t, persisted.DeletedAt.Valid)
		})
	}
}

func TestOrganizationMemberDeletionAllowsSuccessfulAsyncBilling(t *testing.T) {
	for _, hardDelete := range []bool{false, true} {
		name := "soft delete"
		if hardDelete {
			name = "hard delete"
		}
		t.Run(name, func(t *testing.T) {
			db := setupUserOrganizationLifecycleTestDB(t)
			owner := User{Username: "success-owner", Password: "password", Status: common.UserStatusEnabled, AffCode: "success-owner-aff"}
			require.NoError(t, db.Create(&owner).Error)
			organization := createUserLifecycleOrganization(t, db, &owner)
			member := User{
				Username: "success-member", Password: "password", Role: common.RoleCommonUser,
				Status: common.UserStatusEnabled, AffCode: "success-member-aff", AuthVersion: 1,
				OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
				OrganizationStatus: OrganizationMemberStatusActive,
			}
			require.NoError(t, db.Create(&member).Error)
			require.NoError(t, db.Create(&OrganizationMemberFund{OrganizationId: organization.Id, UserId: member.Id}).Error)
			require.NoError(t, db.Create(&Midjourney{
				UserId: member.Id, OrganizationId: organization.Id, MjId: "successful-midjourney",
				Status: "SUCCESS", Progress: "100%", Quota: 10,
				BillingStatus: MidjourneyBillingStatusSettled,
			}).Error)
			require.NoError(t, db.Create(&Task{
				UserId: member.Id, OrganizationId: organization.Id, TaskID: "successful-task",
				Status: TaskStatusSuccess, Progress: "100%", Quota: 10,
			}).Error)

			if hardDelete {
				require.NoError(t, member.HardDelete())
				var count int64
				require.NoError(t, db.Unscoped().Model(&User{}).Where("id = ?", member.Id).Count(&count).Error)
				assert.Zero(t, count)
				return
			}
			require.NoError(t, member.Delete())
			var persisted User
			require.NoError(t, db.Unscoped().First(&persisted, member.Id).Error)
			assert.True(t, persisted.DeletedAt.Valid)
		})
	}
}
