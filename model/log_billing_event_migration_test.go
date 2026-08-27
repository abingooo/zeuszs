package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logBeforeBillingEventMigration struct {
	Id      int
	UserId  int
	Content string
}

func (logBeforeBillingEventMigration) TableName() string {
	return "logs"
}

func TestPrepareSQLiteLogBillingEventColumnsSupportsLegacyUpgradeTwice(t *testing.T) {
	db := openBillingLogOutboxSQLite(t, "legacy-log.db")
	require.NoError(t, db.AutoMigrate(&logBeforeBillingEventMigration{}))
	require.NoError(t, db.Create(&logBeforeBillingEventMigration{UserId: 7, Content: "legacy"}).Error)
	assert.False(t, db.Migrator().HasColumn(&Log{}, "billing_event_id"))

	for range 2 {
		require.NoError(t, prepareSQLiteLogBillingEventColumns(db))
		require.NoError(t, db.AutoMigrate(&Log{}))
	}

	assert.True(t, db.Migrator().HasColumn(&Log{}, "billing_event_id"))
	assert.True(t, db.Migrator().HasColumn(&Log{}, "billing_event_fingerprint"))
	assert.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_billing_event_id"))

	var legacy Log
	require.NoError(t, db.First(&legacy, 1).Error)
	assert.Equal(t, "legacy", legacy.Content)
	assert.Nil(t, legacy.BillingEventId)
	assert.Nil(t, legacy.BillingEventFingerprint)

	eventID := "billing:event:sqlite-upgrade"
	fingerprint := "07c6a63f0e89b681b3b721f852cc4536b94e47dc4a5166b8982c33884e881474"
	require.NoError(t, db.Create(&Log{UserId: 8, BillingEventId: &eventID, BillingEventFingerprint: &fingerprint}).Error)
	err := db.Create(&Log{UserId: 9, BillingEventId: &eventID, BillingEventFingerprint: &fingerprint}).Error
	require.Error(t, err)
}
