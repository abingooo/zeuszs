package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOrganizationAuthorizationModelTest(t *testing.T) (*Organization, []*User) {
	t.Helper()
	previousDB, previousRedis := DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Organization{}, &User{}, &UserSession{}, &Token{}))
	DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedis
	})

	organization := &Organization{
		Name:          "Authorization Organization",
		Status:        OrganizationStatusActive,
		OwnerUserId:   2001,
		PolicyVersion: 1,
	}
	require.NoError(t, db.Create(organization).Error)
	users := []*User{
		{
			Username: "authorization-owner", Password: "password", Status: common.UserStatusEnabled,
			OrganizationId: organization.Id, OrganizationRole: OrganizationRoleOwner,
			OrganizationStatus: OrganizationMemberStatusActive, AuthVersion: 1, AffCode: "authorization-owner-aff",
		},
		{
			Username: "authorization-member", Password: "password", Status: common.UserStatusEnabled,
			OrganizationId: organization.Id, OrganizationRole: OrganizationRoleMember,
			OrganizationStatus: OrganizationMemberStatusActive, AuthVersion: 1, AffCode: "authorization-member-aff",
		},
	}
	for _, user := range users {
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(&UserSession{
			SID: "organization-auth-session-" + user.Username, UserID: user.Id,
			Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive,
			RefreshHash: "organization-auth-refresh-" + user.Username, LoginMethod: "password",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}).Error)
	}
	return organization, users
}

func TestTokenBeforeCreateOverwritesCallerOrganizationSnapshot(t *testing.T) {
	organization, users := setupOrganizationAuthorizationModelTest(t)
	token := &Token{
		UserId:         users[1].Id,
		OrganizationId: organization.Id + 99,
		Key:            "organization-snapshot-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
	}
	require.NoError(t, DB.Create(token).Error)
	assert.Equal(t, organization.Id, token.OrganizationId)
}
