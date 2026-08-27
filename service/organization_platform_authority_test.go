package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationRolesCannotAppointOrganizationAdmin(t *testing.T) {
	db := setupOrganizationManagementTestDB(t)
	owner := createOrganizationManagementUser(t, db, "tenant-role-owner", common.RoleCommonUser, 0, "")
	admin := createOrganizationManagementUser(t, db, "tenant-role-admin", common.RoleCommonUser, 0, "")
	member := createOrganizationManagementUser(t, db, "tenant-role-member", common.RoleCommonUser, 0, "")
	organization := createOrganizationManagementOrganization(t, db, owner.Id, model.OrganizationStatusActive)

	for userID, role := range map[int]model.OrganizationRole{
		owner.Id:  model.OrganizationRoleOwner,
		admin.Id:  model.OrganizationRoleAdmin,
		member.Id: model.OrganizationRoleMember,
	} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"organization_id":     organization.Id,
			"organization_role":   role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}

	for _, actorID := range []int{owner.Id, admin.Id} {
		_, err := AssignOrganizationMemberRoleForPlatform(actorID, AssignOrganizationRoleParams{
			OrganizationID: organization.Id,
			UserID:         member.Id,
			Role:           model.OrganizationRoleAdmin,
		})
		assert.ErrorIs(t, err, ErrPlatformProvisioningForbidden)
	}

	var persistedMember model.User
	require.NoError(t, db.First(&persistedMember, member.Id).Error)
	assert.Equal(t, model.OrganizationRoleMember, persistedMember.OrganizationRole)
}
