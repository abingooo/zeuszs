package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllLogsFiltersAndLabelsOrganizationsAcrossDatabases(t *testing.T) {
	mainDB, logDB := setupBillingLogOutboxTestDatabases(t)
	require.NoError(t, mainDB.AutoMigrate(&Organization{}))

	first := Organization{Name: "First tenant", Status: OrganizationStatusActive, OwnerUserId: 101, PolicyVersion: 1}
	second := Organization{Name: "Second tenant", Status: OrganizationStatusActive, OwnerUserId: 102, PolicyVersion: 1}
	require.NoError(t, mainDB.Create(&first).Error)
	require.NoError(t, mainDB.Create(&second).Error)
	require.NoError(t, logDB.Create(&Log{OrganizationId: first.Id, Type: LogTypeConsume, CreatedAt: 10, Quota: 100}).Error)
	require.NoError(t, logDB.Create(&Log{OrganizationId: second.Id, Type: LogTypeConsume, CreatedAt: 20, Quota: 200}).Error)

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", first.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, first.Id, logs[0].OrganizationId)
	assert.Equal(t, first.Name, logs[0].OrganizationName)

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", "", 0, "", first.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, stat.Quota)
}
