package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyMidjourneyTableMigratesBillingRecoveryColumnsIdempotently(t *testing.T) {
	previousDB := DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})

	require.NoError(t, db.Exec(
		"CREATE TABLE `midjourneys` ("+
			"`id` INTEGER PRIMARY KEY AUTOINCREMENT,"+
			"`user_id` INTEGER,"+
			"`organization_id` INTEGER DEFAULT 0,"+
			"`mj_id` TEXT,"+
			"`submit_time` INTEGER,"+
			"`status` VARCHAR(20),"+
			"`progress` VARCHAR(30),"+
			"`quota` INTEGER,"+
			"`token_id` INTEGER DEFAULT 0,"+
			"`billing_channel_id` INTEGER DEFAULT 0,"+
			"`organization_reservation_id` INTEGER DEFAULT 0"+
			")",
	).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO midjourneys (
			user_id, organization_id, mj_id, submit_time, status, progress, quota,
			token_id, billing_channel_id, organization_reservation_id
		) VALUES (7, 3, 'legacy-midjourney', 1000, 'SUBMITTED', '0%', 120, 9, 11, 13)
	`).Error)

	require.NoError(t, db.AutoMigrate(&Midjourney{}))
	require.NoError(t, db.AutoMigrate(&Midjourney{}))
	for _, column := range []string{
		"billing_token_id",
		"billing_token_reserved",
		"billing_status",
		"upstream_accepted",
	} {
		assert.True(t, db.Migrator().HasColumn(&Midjourney{}, column), column)
	}

	var migrated Midjourney
	require.NoError(t, db.First(&migrated, 1).Error)
	assert.Equal(t, "legacy-midjourney", migrated.MjId)
	assert.Equal(t, 120, migrated.Quota)
	assert.Equal(t, 9, migrated.TokenId)
	assert.Zero(t, migrated.BillingTokenId)
	assert.False(t, migrated.BillingTokenReserved)
	assert.Empty(t, migrated.BillingStatus)
	assert.False(t, migrated.UpstreamAccepted)
	assert.True(t, HasUnfinishedMidjourneyTasks())
}
