package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type taskBeforeBillingRevision struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Quota     int
	CreatedAt int64
	UpdatedAt int64
}

func (taskBeforeBillingRevision) TableName() string {
	return "tasks"
}

func TestMigrateTaskBillingRevisionBackfillsNullBeforeBillingCAS(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE tasks (
			id INTEGER PRIMARY KEY,
			quota INTEGER NOT NULL,
			billing_revision BIGINT NULL,
			updated_at BIGINT NOT NULL DEFAULT 0
		)
	`).Error)
	require.NoError(t, db.Exec("INSERT INTO tasks (id, quota, billing_revision) VALUES (?, ?, NULL)", 1, 120).Error)

	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
	})

	require.NoError(t, migrateTaskBillingRevision())
	var nullCount int64
	require.NoError(t, db.Table("tasks").Where("billing_revision IS NULL").Count(&nullCount).Error)
	assert.Zero(t, nullCount)

	firstPoll := Task{ID: 1, Quota: 120}
	stalePoll := Task{ID: 1, Quota: 120}
	advanced, err := firstPoll.AdvanceBillingQuota(120, 0, 150)
	require.NoError(t, err)
	assert.True(t, advanced)
	advanced, err = stalePoll.AdvanceBillingQuota(120, 0, 150)
	require.NoError(t, err)
	assert.False(t, advanced)

	var persisted struct {
		Quota           int
		BillingRevision int64
	}
	require.NoError(t, db.Table("tasks").Where("id = ?", 1).Take(&persisted).Error)
	assert.Equal(t, 150, persisted.Quota)
	assert.EqualValues(t, 1, persisted.BillingRevision)
}

func TestMigrateTaskBillingRevisionRepairsDirectUpgradeAfterColumnCreation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskBeforeBillingRevision{}))
	require.NoError(t, db.Create(&taskBeforeBillingRevision{ID: 1, Quota: 120}).Error)

	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
	})

	// The pre-AutoMigrate repair is intentionally a no-op when the old schema
	// does not have the column yet. The post-AutoMigrate repair must cover it.
	require.NoError(t, migrateTaskBillingRevision())
	require.NoError(t, db.AutoMigrate(&Task{}))
	require.NoError(t, migrateTaskBillingRevision())

	var persisted struct {
		Quota           int
		BillingRevision *int64
	}
	require.NoError(t, db.Table("tasks").Where("id = ?", 1).Take(&persisted).Error)
	require.NotNil(t, persisted.BillingRevision)
	assert.Zero(t, *persisted.BillingRevision)

	task := Task{ID: 1, Quota: 120}
	advanced, err := task.AdvanceBillingQuota(120, 0, 150)
	require.NoError(t, err)
	assert.True(t, advanced)
}
