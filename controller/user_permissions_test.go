package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/custom_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateUserPermissionsIDVisibility(t *testing.T) {
	originalSetting := custom_setting.IsIDVisibilityEnabled()
	t.Cleanup(func() {
		custom_setting.SetIDVisibilityEnabled(originalSetting)
	})

	tests := []struct {
		name          string
		globalEnabled bool
		platformRole  int
		wantVisible   bool
	}{
		{name: "common user hidden by default", platformRole: common.RoleCommonUser, wantVisible: false},
		{name: "common user follows enabled setting", globalEnabled: true, platformRole: common.RoleCommonUser, wantVisible: true},
		{name: "platform admin always visible", platformRole: common.RoleAdminUser, wantVisible: true},
		{name: "platform root always visible", platformRole: common.RoleRootUser, wantVisible: true},
		{name: "unknown elevated role is not platform admin", platformRole: common.RoleAdminUser + 1, wantVisible: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			custom_setting.SetIDVisibilityEnabled(test.globalEnabled)
			permissions := calculateUserPermissions(test.platformRole)
			visible, ok := permissions["id_visible"].(bool)
			require.True(t, ok)
			assert.Equal(t, test.wantVisible, visible)
		})
	}
}
