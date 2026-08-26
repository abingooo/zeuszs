package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentBrandingEnvironmentOverridesStoredOptions(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalSystemName := common.SystemName
	originalLogo := common.Logo
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		common.SystemName = originalSystemName
		common.Logo = originalLogo
	})

	common.OptionMap = make(map[string]string)
	t.Setenv("SYSTEM_NAME", "  Deployment Name  ")
	t.Setenv("SYSTEM_LOGO", "  /deployment-logo.png  ")

	require.NoError(t, updateOptionMap("SystemName", "Stored Name"))
	require.NoError(t, updateOptionMap("Logo", "/stored-logo.png"))

	assert.Equal(t, "Deployment Name", common.SystemName)
	assert.Equal(t, "Deployment Name", common.OptionMap["SystemName"])
	assert.Equal(t, "/deployment-logo.png", common.Logo)
	assert.Equal(t, "/deployment-logo.png", common.OptionMap["Logo"])
}

func TestDeploymentBrandingUsesStoredOptionsWithoutEnvironment(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalSystemName := common.SystemName
	originalLogo := common.Logo
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		common.SystemName = originalSystemName
		common.Logo = originalLogo
	})

	common.OptionMap = make(map[string]string)
	t.Setenv("SYSTEM_NAME", "")
	t.Setenv("SYSTEM_LOGO", "")

	require.NoError(t, updateOptionMap("SystemName", "Stored Name"))
	require.NoError(t, updateOptionMap("Logo", "/stored-logo.png"))

	assert.Equal(t, "Stored Name", common.SystemName)
	assert.Equal(t, "/stored-logo.png", common.Logo)
}
