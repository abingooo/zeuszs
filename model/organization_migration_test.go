package model

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOrganizationMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalLogDB := LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalLogDSN, hadLogDSN := os.LookupEnv("LOG_SQL_DSN")
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetMainDatabaseType(originalMainType)
		common.SetLogDatabaseType(originalLogType)
		if hadLogDSN {
			require.NoError(t, os.Setenv("LOG_SQL_DSN", originalLogDSN))
		} else {
			require.NoError(t, os.Unsetenv("LOG_SQL_DSN"))
		}
	})
	require.NoError(t, os.Unsetenv("LOG_SQL_DSN"))

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "organization.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)

	require.NoError(t, db.AutoMigrate(
		&Organization{},
		&OrganizationFundAccount{},
		&User{},
		&Token{},
		&Log{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
	))
	return db
}

func setupSeparateOrganizationMigrationLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, os.Setenv("LOG_SQL_DSN", "separate-test-log-database"))
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = logDB
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	return logDB
}

func TestEnsureDefaultOrganizationBackfillsLegacyInstallation(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)

	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&Token{UserId: member.Id, Key: "legacy-token"}).Error)
	require.NoError(t, db.Create(&TopUp{UserId: member.Id, TradeNo: "legacy-order"}).Error)
	require.NoError(t, db.Create(&Task{UserId: member.Id, TaskID: "legacy-task"}).Error)
	require.NoError(t, db.Create(&Midjourney{UserId: member.Id, MjId: "legacy-mj"}).Error)
	require.NoError(t, db.Create(&QuotaData{UserID: member.Id}).Error)
	require.NoError(t, db.Create(&Log{UserId: member.Id, RequestId: "legacy-log"}).Error)
	require.NoError(t, db.Create(&Log{UserId: 0, RequestId: "platform-log"}).Error)
	for _, table := range []string{"users", "tokens", "top_ups", "tasks", "midjourneys", "quota_data", "logs"} {
		require.NoError(t, db.Exec("UPDATE "+table+" SET organization_id = NULL").Error)
	}
	require.NoError(t, db.Exec("UPDATE users SET organization_role = NULL, organization_status = NULL").Error)

	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	organization, err := GetDefaultOrganization()
	require.NoError(t, err)
	require.NotNil(t, organization.SystemKey)
	assert.Equal(t, DefaultOrganizationSystemKey, *organization.SystemKey)
	assert.Equal(t, root.Id, organization.OwnerUserId)
	assert.Equal(t, OrganizationStatusActive, organization.Status)
	assert.True(t, organization.AllowMemberTopup)
	assert.NotZero(t, organization.LegacyMainBackfillAt)
	assert.NotZero(t, organization.LegacyLogBackfillAt)

	require.NoError(t, db.First(&root, root.Id).Error)
	assert.Equal(t, organization.Id, root.OrganizationId)
	assert.Equal(t, OrganizationRoleOwner, root.OrganizationRole)
	assert.Equal(t, OrganizationMemberStatusActive, root.OrganizationStatus)
	require.NoError(t, db.First(&member, member.Id).Error)
	assert.Equal(t, organization.Id, member.OrganizationId)
	assert.Equal(t, OrganizationRoleMember, member.OrganizationRole)
	assert.Equal(t, OrganizationMemberStatusActive, member.OrganizationStatus)

	for _, resource := range []interface{}{
		&Token{}, &TopUp{}, &Task{}, &Midjourney{}, &QuotaData{},
	} {
		var count int64
		require.NoError(t, db.Model(resource).Where("organization_id = ?", organization.Id).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	}
	var legacyTask Task
	require.NoError(t, db.Where("task_id = ?", "legacy-task").First(&legacyTask).Error)
	assert.True(t, legacyTask.LegacyOrganizationWallet)
	var legacyMidjourney Midjourney
	require.NoError(t, db.Where("mj_id = ?", "legacy-mj").First(&legacyMidjourney).Error)
	assert.True(t, legacyMidjourney.LegacyOrganizationWallet)
	var userLog Log
	require.NoError(t, db.Where("request_id = ?", "legacy-log").First(&userLog).Error)
	assert.Equal(t, organization.Id, userLog.OrganizationId)
	var platformLog Log
	require.NoError(t, db.Where("request_id = ?", "platform-log").First(&platformLog).Error)
	assert.Zero(t, platformLog.OrganizationId)

	var account OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&account).Error)
	assert.Zero(t, account.Quota)

	// A second run must not duplicate the tenant or its account.
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	var organizationCount int64
	var accountCount int64
	require.NoError(t, db.Model(&Organization{}).Count(&organizationCount).Error)
	require.NoError(t, db.Model(&OrganizationFundAccount{}).Count(&accountCount).Error)
	assert.Equal(t, int64(1), organizationCount)
	assert.Equal(t, int64(1), accountCount)
}

func TestEnsureDefaultOrganizationRepairsNullableRowsAfterPrematureMarker(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&Token{UserId: member.Id, Key: "missed-null-token"}).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	organization, err := GetDefaultOrganization()
	require.NoError(t, err)
	require.NotZero(t, organization.LegacyMainBackfillAt)

	require.NoError(t, db.Exec("UPDATE users SET organization_id = NULL, organization_role = NULL, organization_status = NULL WHERE id = ?", member.Id).Error)
	require.NoError(t, db.Exec("UPDATE tokens SET organization_id = NULL WHERE user_id = ?", member.Id).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	require.NoError(t, db.First(&member, member.Id).Error)
	assert.Equal(t, organization.Id, member.OrganizationId)
	assert.Equal(t, OrganizationRoleMember, member.OrganizationRole)
	assert.Equal(t, OrganizationMemberStatusActive, member.OrganizationStatus)
	var token Token
	require.NoError(t, db.Where("user_id = ?", member.Id).First(&token).Error)
	assert.Equal(t, organization.Id, token.OrganizationId)

	// Resource snapshots can be missed independently of the user backfill.
	// Marker-driven recovery must remain idempotent even when every user is
	// already assigned to an organization.
	require.NoError(t, db.Exec("UPDATE tokens SET organization_id = NULL WHERE user_id = ?", member.Id).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	require.NoError(t, db.Where("user_id = ?", member.Id).First(&token).Error)
	assert.Equal(t, organization.Id, token.OrganizationId)
}

func TestEnsureDefaultOrganizationBackfillsSoftDeletedTokenSnapshots(t *testing.T) {
	for _, markerWritten := range []bool{false, true} {
		name := "first migration"
		if markerWritten {
			name = "marker repair"
		}
		t.Run(name, func(t *testing.T) {
			db := setupOrganizationMigrationTestDB(t)
			root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
			member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
			require.NoError(t, db.Create(&root).Error)
			require.NoError(t, db.Create(&member).Error)
			token := Token{UserId: member.Id, Key: "soft-deleted-token"}
			require.NoError(t, db.Create(&token).Error)
			if markerWritten {
				require.NoError(t, EnsureDefaultOrganizationAndBackfill())
			}
			require.NoError(t, db.Delete(&token).Error)
			require.NoError(t, db.Exec("UPDATE tokens SET organization_id = NULL WHERE id = ?", token.Id).Error)

			require.NoError(t, EnsureDefaultOrganizationAndBackfill())
			organization, err := GetDefaultOrganization()
			require.NoError(t, err)
			var persisted Token
			require.NoError(t, db.Unscoped().First(&persisted, token.Id).Error)
			assert.Equal(t, organization.Id, persisted.OrganizationId)
			assert.True(t, persisted.DeletedAt.Valid)
		})
	}
}

func TestEnsureDefaultOrganizationRejectsResidualResourceSnapshot(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	token := Token{UserId: member.Id, Key: "orphaned-token"}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	require.NoError(t, db.Unscoped().Delete(&member).Error)
	require.NoError(t, db.Exec("UPDATE tokens SET organization_id = NULL WHERE id = ?", token.Id).Error)

	err := EnsureDefaultOrganizationAndBackfill()
	require.ErrorIs(t, err, ErrOrganizationSnapshotMissing)
	var persisted Token
	require.NoError(t, db.First(&persisted, token.Id).Error)
	assert.Zero(t, persisted.OrganizationId)
}

func TestEnsureDefaultOrganizationRepairsNullableResourceFromOwningOrganization(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	secondOrganization := Organization{
		Name:             "Second Organization",
		Status:           OrganizationStatusActive,
		OwnerUserId:      member.Id,
		AllowMemberTopup: true,
		PolicyVersion:    1,
	}
	require.NoError(t, db.Create(&secondOrganization).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
		"organization_id":     secondOrganization.Id,
		"organization_role":   OrganizationRoleOwner,
		"organization_status": OrganizationMemberStatusActive,
	}).Error)
	token := Token{UserId: member.Id, Key: "second-organization-token"}
	require.NoError(t, db.Create(&token).Error)
	assert.Equal(t, secondOrganization.Id, token.OrganizationId)

	require.NoError(t, db.Exec("UPDATE tokens SET organization_id = NULL WHERE id = ?", token.Id).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, secondOrganization.Id, token.OrganizationId)
}

func TestEnsureDefaultOrganizationMarksOnlyLegacyWalletTasks(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	walletTask := Task{UserId: member.Id, TaskID: "legacy-wallet-task"}
	subscriptionTask := Task{
		UserId: member.Id,
		TaskID: "legacy-subscription-task",
		PrivateData: TaskPrivateData{
			BillingSource:  taskBillingSourceSubscription,
			SubscriptionId: 1,
		},
	}
	require.NoError(t, db.Create(&walletTask).Error)
	require.NoError(t, db.Create(&subscriptionTask).Error)
	require.NoError(t, db.Exec("UPDATE tasks SET organization_id = NULL").Error)

	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	require.NoError(t, db.First(&walletTask, walletTask.ID).Error)
	assert.True(t, walletTask.LegacyOrganizationWallet)
	require.NoError(t, db.First(&subscriptionTask, subscriptionTask.ID).Error)
	assert.False(t, subscriptionTask.LegacyOrganizationWallet)
}

func TestEnsureDefaultOrganizationLeavesEmptySetupDatabaseUntouched(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)

	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	var count int64
	require.NoError(t, db.Model(&Organization{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestLegacyUserInsertMethodsFailClosedAfterDefaultOrganizationProvisioning(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{
		Username: "root", Password: "hashed", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, AffCode: "root-aff",
	}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	direct := User{
		Username: "legacy-direct", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled,
	}
	require.ErrorIs(t, direct.Insert(0), ErrOrganizationLedgerRequired)

	transactional := User{
		Username: "legacy-transactional", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		return transactional.InsertWithTx(tx, 0)
	})
	require.ErrorIs(t, err, ErrOrganizationLedgerRequired)

	var count int64
	require.NoError(t, db.Model(&User{}).
		Where("username IN ?", []string{direct.Username, transactional.Username}).
		Count(&count).Error)
	assert.Zero(t, count)
}

func TestLegacyUserInsertFailsClosedWhenOrganizationProbeFails(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	forcedErr := errors.New("organization probe failed")
	callbackName := "test:fail_organization_probe"
	callbackRegistered := true
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "organizations" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = db.Callback().Query().Remove(callbackName)
		}
	})

	user := User{
		Username: "probe-failure", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled,
	}
	require.ErrorIs(t, user.Insert(0), forcedErr)
	require.NoError(t, db.Callback().Query().Remove(callbackName))
	callbackRegistered = false

	var count int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", user.Username).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEnsureDefaultOrganizationFailsClosedWithoutUniqueRoot(t *testing.T) {
	t.Run("no root", func(t *testing.T) {
		db := setupOrganizationMigrationTestDB(t)
		require.NoError(t, db.Create(&User{
			Username: "member", Password: "hashed", Role: common.RoleCommonUser, AffCode: "member-aff",
		}).Error)

		err := EnsureDefaultOrganizationAndBackfill()
		require.ErrorIs(t, err, ErrOrganizationMigrationNoRoot)
	})

	t.Run("multiple roots", func(t *testing.T) {
		db := setupOrganizationMigrationTestDB(t)
		require.NoError(t, db.Create(&User{Username: "root-1", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-1-aff"}).Error)
		require.NoError(t, db.Create(&User{Username: "root-2", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-2-aff"}).Error)

		err := EnsureDefaultOrganizationAndBackfill()
		require.ErrorIs(t, err, ErrOrganizationMigrationMultipleRoot)
	})
}

func TestEnsureDefaultOrganizationRejectsMissingMembershipAfterBackfill(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	require.NoError(t, db.Create(&User{
		Username: "invalid-member", Password: "hashed", Role: common.RoleCommonUser, AffCode: "invalid-member-aff",
	}).Error)

	err := EnsureDefaultOrganizationAndBackfill()
	require.ErrorIs(t, err, ErrOrganizationMembershipMissing)
}

func TestEnsureDefaultOrganizationRejectsInvalidMembershipIntegrity(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, db *gorm.DB, root User, member User)
		expectedErr error
	}{
		{
			name: "orphan organization",
			mutate: func(t *testing.T, db *gorm.DB, _ User, member User) {
				require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
					"organization_id":     999999,
					"organization_role":   OrganizationRoleMember,
					"organization_status": OrganizationMemberStatusActive,
				}).Error)
			},
			expectedErr: ErrOrganizationMembershipInvalid,
		},
		{
			name: "invalid role",
			mutate: func(t *testing.T, db *gorm.DB, _ User, member User) {
				require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Update("organization_role", "invalid").Error)
			},
			expectedErr: ErrOrganizationMembershipInvalid,
		},
		{
			name: "empty status",
			mutate: func(t *testing.T, db *gorm.DB, _ User, member User) {
				require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Update("organization_status", "").Error)
			},
			expectedErr: ErrOrganizationMembershipInvalid,
		},
		{
			name: "organization owner mismatch",
			mutate: func(t *testing.T, db *gorm.DB, _ User, member User) {
				require.NoError(t, db.Create(&Organization{
					Name: "Ownerless Organization", Status: OrganizationStatusActive,
					OwnerUserId: member.Id, PolicyVersion: 1,
				}).Error)
			},
			expectedErr: ErrOrganizationOwnerInvalid,
		},
		{
			name: "disabled organization owner",
			mutate: func(t *testing.T, db *gorm.DB, _ User, member User) {
				organization := Organization{
					Name: "Disabled Owner Organization", Status: OrganizationStatusActive,
					OwnerUserId: member.Id, PolicyVersion: 1,
				}
				require.NoError(t, db.Create(&organization).Error)
				require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
					"organization_id":     organization.Id,
					"organization_role":   OrganizationRoleOwner,
					"organization_status": OrganizationMemberStatusDisabled,
				}).Error)
			},
			expectedErr: ErrOrganizationOwnerInvalid,
		},
		{
			name: "default root mismatch",
			mutate: func(t *testing.T, db *gorm.DB, root User, _ User) {
				require.NoError(t, db.Model(&User{}).Where("id = ?", root.Id).Update("organization_role", OrganizationRoleAdmin).Error)
			},
			expectedErr: ErrDefaultOrganizationConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupOrganizationMigrationTestDB(t)
			root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}
			member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "member-aff"}
			require.NoError(t, db.Create(&root).Error)
			require.NoError(t, db.Create(&member).Error)
			require.NoError(t, EnsureDefaultOrganizationAndBackfill())
			test.mutate(t, db, root, member)

			err := EnsureDefaultOrganizationAndBackfill()
			require.ErrorIs(t, err, test.expectedErr)
		})
	}
}

func TestEnsureDefaultOrganizationForRootTxRollsBackAtomically(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
	require.NoError(t, db.Create(&root).Error)
	sentinel := errors.New("rollback")

	err := db.Transaction(func(tx *gorm.DB) error {
		organization, err := EnsureDefaultOrganizationForRootTx(tx, root.Id)
		require.NoError(t, err)
		require.NotZero(t, organization.Id)
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	var organizationCount int64
	require.NoError(t, db.Model(&Organization{}).Count(&organizationCount).Error)
	assert.Zero(t, organizationCount)
	require.NoError(t, db.First(&root, root.Id).Error)
	assert.Zero(t, root.OrganizationId)
}

func TestDefaultOrganizationSystemKeyIsUniqueAndNullable(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())

	require.NoError(t, db.Create(&Organization{
		Name: "Organization A", Status: OrganizationStatusActive, OwnerUserId: 1001, PolicyVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&Organization{
		Name: "Organization B", Status: OrganizationStatusActive, OwnerUserId: 1002, PolicyVersion: 1,
	}).Error)
	systemKey := DefaultOrganizationSystemKey
	err := db.Create(&Organization{
		Name: "Conflicting Default", SystemKey: &systemKey, Status: OrganizationStatusActive,
		OwnerUserId: 1003, PolicyVersion: 1,
	}).Error
	require.Error(t, err)
}

func TestBackfillLegacyOrganizationSnapshotsInSeparateLogDatabaseByUserOrganization(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	require.NoError(t, os.Setenv("LOG_SQL_DSN", "separate-test-log-database"))
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
	member := User{Username: "member", Password: "hashed", Role: common.RoleCommonUser, AffCode: "member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	organization, err := GetDefaultOrganization()
	require.NoError(t, err)
	assert.Zero(t, organization.LegacyLogBackfillAt)

	secondOrganization := Organization{
		Name: "Second Organization", Status: OrganizationStatusActive,
		OwnerUserId: member.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&secondOrganization).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Updates(map[string]interface{}{
		"organization_id":     secondOrganization.Id,
		"organization_role":   OrganizationRoleOwner,
		"organization_status": OrganizationMemberStatusActive,
	}).Error)

	logDB := setupSeparateOrganizationMigrationLogDB(t)
	require.NoError(t, logDB.Create(&Log{UserId: root.Id, RequestId: "default-organization-log"}).Error)
	require.NoError(t, logDB.Create(&Log{UserId: member.Id, RequestId: "second-organization-log"}).Error)
	require.NoError(t, logDB.Create(&Log{UserId: 0, RequestId: "legacy-platform-log"}).Error)
	require.NoError(t, logDB.Exec("UPDATE logs SET organization_id = NULL").Error)

	require.NoError(t, backfillLegacyLogOrganization())

	var defaultOrganizationLog Log
	require.NoError(t, logDB.Where("request_id = ?", "default-organization-log").First(&defaultOrganizationLog).Error)
	assert.Equal(t, organization.Id, defaultOrganizationLog.OrganizationId)
	var secondOrganizationLog Log
	require.NoError(t, logDB.Where("request_id = ?", "second-organization-log").First(&secondOrganizationLog).Error)
	assert.Equal(t, secondOrganization.Id, secondOrganizationLog.OrganizationId)
	var platformLog Log
	require.NoError(t, logDB.Where("request_id = ?", "legacy-platform-log").First(&platformLog).Error)
	assert.Zero(t, platformLog.OrganizationId)
	require.NoError(t, db.First(&organization, organization.Id).Error)
	assert.NotZero(t, organization.LegacyLogBackfillAt)
}

func TestBackfillLegacyOrganizationSnapshotsRepairsPrematureMarkerFromSoftDeletedUser(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	require.NoError(t, os.Setenv("LOG_SQL_DSN", "separate-test-log-database"))
	root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
	owner := User{Username: "owner", Password: "hashed", Role: common.RoleCommonUser, AffCode: "owner-aff"}
	archivedMember := User{Username: "archived-member", Password: "hashed", Role: common.RoleCommonUser, AffCode: "archived-member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&archivedMember).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	defaultOrganization, err := GetDefaultOrganization()
	require.NoError(t, err)

	secondOrganization := Organization{
		Name: "Second Organization", Status: OrganizationStatusActive,
		OwnerUserId: owner.Id, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&secondOrganization).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id":     secondOrganization.Id,
		"organization_role":   OrganizationRoleOwner,
		"organization_status": OrganizationMemberStatusActive,
	}).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", archivedMember.Id).Update("organization_id", secondOrganization.Id).Error)
	require.NoError(t, db.Delete(&archivedMember).Error)

	logDB := setupSeparateOrganizationMigrationLogDB(t)
	require.NoError(t, logDB.Create(&Log{UserId: archivedMember.Id, RequestId: "soft-deleted-user-log"}).Error)
	require.NoError(t, logDB.Exec("UPDATE logs SET organization_id = NULL").Error)
	const prematureMarker int64 = 123456
	require.NoError(t, db.Model(&Organization{}).Where("id = ?", defaultOrganization.Id).
		Update("legacy_log_backfill_at", prematureMarker).Error)

	require.NoError(t, backfillLegacyLogOrganization())

	var archivedMemberLog Log
	require.NoError(t, logDB.Where("request_id = ?", "soft-deleted-user-log").First(&archivedMemberLog).Error)
	assert.Equal(t, secondOrganization.Id, archivedMemberLog.OrganizationId)
	require.NoError(t, db.First(&defaultOrganization, defaultOrganization.Id).Error)
	assert.Equal(t, prematureMarker, defaultOrganization.LegacyLogBackfillAt)
}

func TestBackfillLegacyOrganizationSnapshotsInSeparateLogDatabaseRejectsUnknownUser(t *testing.T) {
	for _, test := range []struct {
		name   string
		marker int64
	}{
		{name: "without marker"},
		{name: "with premature marker", marker: 123456},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupOrganizationMigrationTestDB(t)
			require.NoError(t, os.Setenv("LOG_SQL_DSN", "separate-test-log-database"))
			root := User{Username: "root", Password: "hashed", Role: common.RoleRootUser, AffCode: "root-aff"}
			require.NoError(t, db.Create(&root).Error)
			require.NoError(t, EnsureDefaultOrganizationAndBackfill())
			organization, err := GetDefaultOrganization()
			require.NoError(t, err)
			if test.marker != 0 {
				require.NoError(t, db.Model(&Organization{}).Where("id = ?", organization.Id).
					Update("legacy_log_backfill_at", test.marker).Error)
			}

			logDB := setupSeparateOrganizationMigrationLogDB(t)
			require.NoError(t, logDB.Create(&Log{UserId: root.Id, RequestId: "known-user-log"}).Error)
			require.NoError(t, logDB.Create(&Log{UserId: 999999, RequestId: "unknown-user-log"}).Error)
			require.NoError(t, logDB.Exec("UPDATE logs SET organization_id = NULL").Error)

			err = backfillLegacyLogOrganization()
			require.ErrorIs(t, err, ErrOrganizationLogBackfillIncomplete)
			require.NoError(t, db.First(&organization, organization.Id).Error)
			assert.Equal(t, test.marker, organization.LegacyLogBackfillAt)
			var knownUserLog Log
			require.NoError(t, logDB.Where("request_id = ?", "known-user-log").First(&knownUserLog).Error)
			assert.Equal(t, organization.Id, knownUserLog.OrganizationId)
			var unknownUserLog Log
			require.NoError(t, logDB.Where("request_id = ?", "unknown-user-log").First(&unknownUserLog).Error)
			assert.Zero(t, unknownUserLog.OrganizationId)
		})
	}
}
