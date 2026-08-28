package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/custom_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIDVisibilityOptionRegistrationAndRuntimeUpdate(t *testing.T) {
	originalDB := DB
	originalOptionMap := common.OptionMap
	originalSetting := custom_setting.IsIDVisibilityEnabled()
	t.Cleanup(func() {
		DB = originalDB
		common.OptionMap = originalOptionMap
		custom_setting.SetIDVisibilityEnabled(originalSetting)
	})
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	t.Cleanup(func() {
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	common.OptionMap = make(map[string]string)
	custom_setting.SetIDVisibilityEnabled(false)

	exported := config.GlobalConfig.ExportAllConfigs()
	require.Contains(t, exported, "custom_setting.id_visibility_enabled")
	assert.Equal(t, "false", exported["custom_setting.id_visibility_enabled"])

	require.NoError(t, UpdateOption("custom_setting.id_visibility_enabled", "true"))
	var stored Option
	require.NoError(t, db.Where("key = ?", "custom_setting.id_visibility_enabled").First(&stored).Error)
	assert.Equal(t, "true", stored.Value)
	assert.True(t, custom_setting.IsIDVisibilityEnabled())
	assert.Equal(t, "true", common.OptionMap["custom_setting.id_visibility_enabled"])

	custom_setting.SetIDVisibilityEnabled(false)
	common.OptionMap["custom_setting.id_visibility_enabled"] = "false"
	loadOptionsFromDatabase()
	assert.True(t, custom_setting.IsIDVisibilityEnabled())
	assert.Equal(t, "true", common.OptionMap["custom_setting.id_visibility_enabled"])

	err = UpdateOption("custom_setting.id_visibility_enabled", "invalid")
	require.Error(t, err)
	require.NoError(t, db.Where("key = ?", "custom_setting.id_visibility_enabled").First(&stored).Error)
	assert.Equal(t, "true", stored.Value)
	assert.True(t, custom_setting.IsIDVisibilityEnabled())
	assert.Equal(t, "true", common.OptionMap["custom_setting.id_visibility_enabled"])
}
