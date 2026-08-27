package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceCreateHooksOverwriteForgedOrganizationSnapshot(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "snapshot-root", Password: "hashed", Role: common.RoleRootUser, AffCode: "snapshot-root-aff"}
	member := User{Username: "snapshot-member", Password: "hashed", Role: common.RoleCommonUser, AffCode: "snapshot-member-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	organization, err := GetDefaultOrganization()
	require.NoError(t, err)

	forgedOrganizationID := organization.Id + 100
	task := &Task{UserId: member.Id, OrganizationId: forgedOrganizationID, TaskID: "snapshot-task"}
	require.NoError(t, db.Create(task).Error)
	assert.Equal(t, organization.Id, task.OrganizationId)

	midjourney := &Midjourney{UserId: member.Id, OrganizationId: forgedOrganizationID, MjId: "snapshot-mj"}
	require.NoError(t, db.Create(midjourney).Error)
	assert.Equal(t, organization.Id, midjourney.OrganizationId)

	topUp := &TopUp{UserId: member.Id, OrganizationId: forgedOrganizationID, TradeNo: "snapshot-topup"}
	require.NoError(t, db.Create(topUp).Error)
	assert.Equal(t, organization.Id, topUp.OrganizationId)

	quotaData := &QuotaData{UserID: member.Id, OrganizationID: forgedOrganizationID, Username: member.Username, ModelName: "snapshot-model"}
	require.NoError(t, db.Create(quotaData).Error)
	assert.Equal(t, organization.Id, quotaData.OrganizationID)
}

func TestCreateLogAndQuotaDataRejectMissingTenantAfterProvisioning(t *testing.T) {
	db := setupOrganizationMigrationTestDB(t)
	root := User{Username: "snapshot-root-2", Password: "hashed", Role: common.RoleRootUser, AffCode: "snapshot-root-2-aff"}
	member := User{Username: "snapshot-member-2", Password: "hashed", Role: common.RoleCommonUser, AffCode: "snapshot-member-2-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, EnsureDefaultOrganizationAndBackfill())
	organization, err := GetDefaultOrganization()
	require.NoError(t, err)
	require.NoError(t, db.Model(&User{}).Where("id = ?", member.Id).Update("organization_id", 0).Error)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	log := &Log{UserId: member.Id, OrganizationId: organization.Id, RequestId: "missing-tenant-log"}
	require.ErrorIs(t, createLog(log), ErrOrganizationSnapshotInvalid)
	assert.Zero(t, log.OrganizationId)

	LogQuotaData(QuotaDataLogParams{UserID: member.Id, OrganizationID: organization.Id, Username: member.Username, ModelName: "missing-tenant-model"})
	CacheQuotaDataLock.Lock()
	assert.Empty(t, CacheQuotaData)
	CacheQuotaDataLock.Unlock()
}
