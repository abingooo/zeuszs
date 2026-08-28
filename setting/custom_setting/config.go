package custom_setting

import (
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/QuantumNous/new-api/setting/config"
)

const IDVisibilityOptionKey = "custom_setting.id_visibility_enabled"

type CustomSetting struct {
	idVisibilityEnabled atomic.Bool
}

var customSetting CustomSetting

func init() {
	config.GlobalConfig.Register("custom_setting", &customSetting)
}

func (s *CustomSetting) ExportConfigValues() (map[string]string, error) {
	return map[string]string{
		"id_visibility_enabled": strconv.FormatBool(s.idVisibilityEnabled.Load()),
	}, nil
}

func (s *CustomSetting) ApplyConfigValues(values map[string]string) error {
	value, ok := values["id_visibility_enabled"]
	if !ok {
		return nil
	}
	switch value {
	case "true":
		s.idVisibilityEnabled.Store(true)
	case "false":
		s.idVisibilityEnabled.Store(false)
	default:
		return fmt.Errorf("id_visibility_enabled must be true or false")
	}
	return nil
}

func SetIDVisibilityEnabled(enabled bool) {
	customSetting.idVisibilityEnabled.Store(enabled)
}

func IsIDVisibilityEnabled() bool {
	return customSetting.idVisibilityEnabled.Load()
}
