package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRegistrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Organization{},
		&model.OrganizationInvite{},
		&model.OrganizationInviteUse{},
		&model.OrganizationFundAccount{},
		&model.OrganizationMemberFund{},
		&model.OrganizationQuotaLedger{},
		&model.OrganizationQuotaOperation{},
		&model.OrganizationWalletReservation{},
		&model.OrganizationAuditEvent{},
		&model.BillingLogOutbox{},
		&model.User{},
		&model.Token{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousNewUserQuota := common.QuotaForNewUser
	previousInviteeQuota := common.QuotaForInvitee
	previousInviterQuota := common.QuotaForInviter
	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.QuotaForNewUser = 1200
	common.QuotaForInvitee = 0
	common.QuotaForInviter = 0
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		common.QuotaForNewUser = previousNewUserQuota
		common.QuotaForInvitee = previousInviteeQuota
		common.QuotaForInviter = previousInviterQuota
	})
	return db
}

func createRegistrationTestOrganization(t *testing.T, db *gorm.DB, name string, systemKey *string, ownerID int) model.Organization {
	t.Helper()
	organization := model.Organization{
		Name:                 name,
		SystemKey:            systemKey,
		Status:               model.OrganizationStatusActive,
		OwnerUserId:          ownerID,
		AllowMemberTopup:     true,
		PolicyVersion:        1,
		LegacyMainBackfillAt: 1,
		LegacyLogBackfillAt:  1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	return organization
}

func TestRegisterUserClaimsOrganizationInviteAndCreatesAuditedWallet(t *testing.T) {
	db := setupRegistrationTestDB(t)
	defaultKey := model.DefaultOrganizationSystemKey
	_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)
	invitedOrganization := createRegistrationTestOrganization(t, db, "Research Group", nil, 200)
	invite := model.OrganizationInvite{
		OrganizationId: invitedOrganization.Id,
		CodeHash:       HashOrganizationInviteCode("LAB-ACCESS"),
		CodePrefix:     "LAB",
		Status:         model.OrganizationInviteStatusActive,
		MaxUses:        1,
		DefaultRole:    model.OrganizationRoleMember,
		CreatedBy:      200,
	}
	require.NoError(t, db.Create(&invite).Error)

	registration, err := RegisterUser(UserRegistrationParams{
		User: &model.User{
			Username:    "invitee",
			Password:    "password123",
			DisplayName: "Invitee",
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		},
		OrganizationInviteCode: " lab-access ",
		RequestID:              "registration-request-1",
		GenerateDefaultToken:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, registration)
	assert.Equal(t, invitedOrganization.Id, registration.User.OrganizationId)
	assert.Equal(t, model.OrganizationRoleMember, registration.User.OrganizationRole)
	assert.Equal(t, 1200, registration.User.Quota)

	var persistedUser model.User
	require.NoError(t, db.First(&persistedUser, registration.User.Id).Error)
	assert.Equal(t, invitedOrganization.Id, persistedUser.OrganizationId)
	assert.Equal(t, 1200, persistedUser.Quota)

	var token model.Token
	require.NoError(t, db.Where("user_id = ?", registration.User.Id).First(&token).Error)
	assert.Equal(t, invitedOrganization.Id, token.OrganizationId)

	var persistedInvite model.OrganizationInvite
	require.NoError(t, db.First(&persistedInvite, invite.Id).Error)
	assert.Equal(t, 1, persistedInvite.UsedCount)
	var inviteUse model.OrganizationInviteUse
	require.NoError(t, db.Where("user_id = ?", registration.User.Id).First(&inviteUse).Error)
	assert.Equal(t, invitedOrganization.Id, inviteUse.OrganizationId)

	var ledger model.OrganizationQuotaLedger
	require.NoError(t, db.Where("idempotency_key = ?", "registration:"+strconv.Itoa(registration.User.Id)+":initial").First(&ledger).Error)
	assert.Equal(t, int64(1200), ledger.UserQuotaDelta)
	assert.Equal(t, int64(1200), ledger.UserQuotaAfter)
	assert.Zero(t, ledger.RecoverableQuotaAfter)
}

func TestRegisterUserRejectsOrganizationInviteWithElevatedRole(t *testing.T) {
	for _, role := range []model.OrganizationRole{model.OrganizationRoleAdmin, model.OrganizationRoleOwner} {
		t.Run(string(role), func(t *testing.T) {
			db := setupRegistrationTestDB(t)
			defaultKey := model.DefaultOrganizationSystemKey
			_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)
			organization := createRegistrationTestOrganization(t, db, "Tampered Invite", nil, 200)
			invite := model.OrganizationInvite{
				OrganizationId: organization.Id,
				CodeHash:       HashOrganizationInviteCode("ELEVATED-" + string(role)),
				CodePrefix:     "ELEVATED",
				Status:         model.OrganizationInviteStatusActive,
				MaxUses:        1,
				DefaultRole:    role,
				CreatedBy:      200,
			}
			require.NoError(t, db.Create(&invite).Error)

			_, err := RegisterUser(UserRegistrationParams{
				User: &model.User{
					Username:    "elevated-" + string(role),
					Password:    "password123",
					DisplayName: "Elevated " + string(role),
					Role:        common.RoleCommonUser,
					Status:      common.UserStatusEnabled,
				},
				OrganizationInviteCode: "ELEVATED-" + string(role),
				RequestID:              "elevated-invite-" + string(role),
				GenerateDefaultToken:   true,
			})
			require.ErrorIs(t, err, ErrOrganizationInviteInvalid)

			var userCount, tokenCount, inviteUseCount, ledgerCount int64
			require.NoError(t, db.Model(&model.User{}).Count(&userCount).Error)
			require.NoError(t, db.Model(&model.Token{}).Count(&tokenCount).Error)
			require.NoError(t, db.Model(&model.OrganizationInviteUse{}).Count(&inviteUseCount).Error)
			require.NoError(t, db.Model(&model.OrganizationQuotaLedger{}).Count(&ledgerCount).Error)
			assert.Zero(t, userCount)
			assert.Zero(t, tokenCount)
			assert.Zero(t, inviteUseCount)
			assert.Zero(t, ledgerCount)

			var persistedInvite model.OrganizationInvite
			require.NoError(t, db.First(&persistedInvite, invite.Id).Error)
			assert.Zero(t, persistedInvite.UsedCount)
		})
	}
}

func TestConcurrentRegistrationsCanShareOrganizationInviteCapacity(t *testing.T) {
	db := setupRegistrationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A single SQLite connection serializes the database transactions while the
	// registration calls still race at the application boundary. Row-locking
	// databases serialize them on the organization scope instead.
	sqlDB.SetMaxOpenConns(1)

	defaultKey := model.DefaultOrganizationSystemKey
	_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)
	organization := createRegistrationTestOrganization(t, db, "Shared Invite", nil, 200)
	invite := model.OrganizationInvite{
		OrganizationId: organization.Id,
		CodeHash:       HashOrganizationInviteCode("TWO-MEMBERS"),
		CodePrefix:     "TWO",
		Status:         model.OrganizationInviteStatusActive,
		MaxUses:        2,
		DefaultRole:    model.OrganizationRoleMember,
		CreatedBy:      200,
	}
	require.NoError(t, db.Create(&invite).Error)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := RegisterUser(UserRegistrationParams{
				User: &model.User{
					Username:    fmt.Sprintf("concurrent-invitee-%d", index),
					Password:    "password123",
					DisplayName: fmt.Sprintf("Concurrent Invitee %d", index),
					Role:        common.RoleCommonUser,
					Status:      common.UserStatusEnabled,
				},
				OrganizationInviteCode: "TWO-MEMBERS",
				RequestID:              fmt.Sprintf("concurrent-registration-%d", index),
			})
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var persistedInvite model.OrganizationInvite
	require.NoError(t, db.First(&persistedInvite, invite.Id).Error)
	assert.Equal(t, 2, persistedInvite.UsedCount)
	var memberCount int64
	require.NoError(t, db.Model(&model.User{}).Where("organization_id = ?", organization.Id).Count(&memberCount).Error)
	assert.EqualValues(t, 2, memberCount)
	var useCount int64
	require.NoError(t, db.Model(&model.OrganizationInviteUse{}).Where("organization_invite_id = ?", invite.Id).Count(&useCount).Error)
	assert.EqualValues(t, 2, useCount)
}

func TestClaimOrganizationInviteUsesCurrentCountAfterStaleRead(t *testing.T) {
	db := setupRegistrationTestDB(t)
	defaultKey := model.DefaultOrganizationSystemKey
	_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)
	organization := createRegistrationTestOrganization(t, db, "Stale Invite", nil, 200)
	invite := model.OrganizationInvite{
		OrganizationId: organization.Id,
		CodeHash:       HashOrganizationInviteCode("STALE-COUNT"),
		CodePrefix:     "STALE",
		Status:         model.OrganizationInviteStatusActive,
		MaxUses:        2,
		DefaultRole:    model.OrganizationRoleMember,
		CreatedBy:      200,
	}
	require.NoError(t, db.Create(&invite).Error)

	staleInvite := invite
	require.NoError(t, db.Model(&model.OrganizationInvite{}).Where("id = ?", invite.Id).Update("used_count", 1).Error)
	user := model.User{
		Username:           "stale-count-invitee",
		Password:           "password123",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		OrganizationId:     organization.Id,
		OrganizationRole:   model.OrganizationRoleMember,
		OrganizationStatus: model.OrganizationMemberStatusActive,
	}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := claimOrganizationInviteTx(tx, &staleInvite, user.Id, "stale-count-registration")
		return err
	}))

	var persistedInvite model.OrganizationInvite
	require.NoError(t, db.First(&persistedInvite, invite.Id).Error)
	assert.Equal(t, 2, persistedInvite.UsedCount)
}

func TestRegisterUserRollsBackWhenOrganizationInviteIsExhausted(t *testing.T) {
	db := setupRegistrationTestDB(t)
	defaultKey := model.DefaultOrganizationSystemKey
	_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)
	organization := createRegistrationTestOrganization(t, db, "Enterprise", nil, 200)
	invite := model.OrganizationInvite{
		OrganizationId: organization.Id,
		CodeHash:       HashOrganizationInviteCode("ONE-USE"),
		CodePrefix:     "ONE",
		Status:         model.OrganizationInviteStatusActive,
		MaxUses:        1,
		UsedCount:      1,
		DefaultRole:    model.OrganizationRoleMember,
		CreatedBy:      200,
	}
	require.NoError(t, db.Create(&invite).Error)

	_, err := RegisterUser(UserRegistrationParams{
		User: &model.User{
			Username:    "rejected",
			Password:    "password123",
			DisplayName: "Rejected",
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		},
		OrganizationInviteCode: "ONE-USE",
		RequestID:              "registration-request-2",
		GenerateDefaultToken:   true,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOrganizationInviteExhausted))

	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "rejected").Count(&userCount).Error)
	assert.Zero(t, userCount)
	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Count(&tokenCount).Error)
	assert.Zero(t, tokenCount)
	var ledgerCount int64
	require.NoError(t, db.Model(&model.OrganizationQuotaLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount)
}

func TestRegisterUserRollsBackAllPersistentStateWhenProviderBindingFails(t *testing.T) {
	db := setupRegistrationTestDB(t)
	defaultKey := model.DefaultOrganizationSystemKey
	_ = createRegistrationTestOrganization(t, db, "Default", &defaultKey, 100)

	expected := errors.New("provider binding failed")
	_, err := RegisterUser(UserRegistrationParams{
		User: &model.User{
			Username:    "oauth-user",
			DisplayName: "OAuth User",
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		},
		RequestID:            "registration-request-3",
		GenerateDefaultToken: true,
		AfterCreateTx: func(_ *gorm.DB, _ *model.User) error {
			return expected
		},
	})
	require.ErrorIs(t, err, expected)

	for _, table := range []interface{}{
		&model.User{},
		&model.Token{},
		&model.OrganizationMemberFund{},
		&model.OrganizationQuotaLedger{},
		&model.OrganizationQuotaOperation{},
	} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
}

func TestProvisionRootUserWithTxCreatesOrganizationAndAuditedWallet(t *testing.T) {
	db := setupRegistrationTestDB(t)
	root := &model.User{
		Username:    "setup-root",
		Password:    "password123",
		DisplayName: "Root User",
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ProvisionRootUserWithTx(tx, root, 100000000, "setup-request-1")
	}))

	assert.NotZero(t, root.Id)
	assert.Equal(t, model.OrganizationRoleOwner, root.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, root.OrganizationStatus)
	assert.Equal(t, 100000000, root.Quota)

	organization, err := model.GetDefaultOrganization()
	require.NoError(t, err)
	assert.Equal(t, root.Id, organization.OwnerUserId)
	var fund model.OrganizationMemberFund
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, root.Id).First(&fund).Error)
	var ledger model.OrganizationQuotaLedger
	require.NoError(t, db.Where("idempotency_key = ?", "setup:"+strconv.Itoa(root.Id)+":root-grant").First(&ledger).Error)
	assert.Equal(t, int64(100000000), ledger.UserQuotaDelta)
	assert.Equal(t, int64(100000000), ledger.UserQuotaAfter)
}

func TestProvisionRootUserWithTxRollsBackOrganizationAndWallet(t *testing.T) {
	db := setupRegistrationTestDB(t)
	root := &model.User{
		Username: "setup-root-rollback", Password: "password123",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled,
	}
	sentinel := errors.New("rollback setup")
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, ProvisionRootUserWithTx(tx, root, 100000000, "setup-request-rollback"))
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	var userCount, organizationCount, ledgerCount int64
	require.NoError(t, db.Model(&model.User{}).Count(&userCount).Error)
	require.NoError(t, db.Model(&model.Organization{}).Count(&organizationCount).Error)
	require.NoError(t, db.Model(&model.OrganizationQuotaLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, userCount)
	assert.Zero(t, organizationCount)
	assert.Zero(t, ledgerCount)
}
