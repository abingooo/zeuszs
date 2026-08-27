package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPostSetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousSetup := constant.Setup
	previousRedis := common.RedisEnabled
	previousOptionMap := common.OptionMap
	previousSelfUse := operation_setting.SelfUseModeEnabled
	previousDemo := operation_setting.DemoSiteEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Organization{},
		&model.OrganizationInvite{},
		&model.OrganizationInviteUse{},
		&model.OrganizationFundAccount{},
		&model.OrganizationMemberFund{},
		&model.OrganizationQuotaLedger{},
		&model.OrganizationQuotaOperation{},
		&model.OrganizationWalletReservation{},
		&model.OrganizationAuditEvent{},
		&model.User{},
		&model.Token{},
		&model.Option{},
		&model.Setup{},
	))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.OptionMap = make(map[string]string)
	constant.Setup = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
		common.OptionMap = previousOptionMap
		constant.Setup = previousSetup
		operation_setting.SelfUseModeEnabled = previousSelfUse
		operation_setting.DemoSiteEnabled = previousDemo
	})
	return db
}

func TestPostSetupProvisionsDefaultOrganizationAndRootLedger(t *testing.T) {
	db := setupPostSetupTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(`{"username":"root","password":"password123","confirmPassword":"password123"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	PostSetup(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.True(t, constant.Setup)

	var root model.User
	require.NoError(t, db.Where("role = ?", common.RoleRootUser).First(&root).Error)
	assert.NotZero(t, root.OrganizationId)
	assert.Equal(t, model.OrganizationRoleOwner, root.OrganizationRole)
	assert.Equal(t, model.OrganizationMemberStatusActive, root.OrganizationStatus)
	assert.Equal(t, 100000000, root.Quota)

	organization, err := model.GetDefaultOrganization()
	require.NoError(t, err)
	assert.Equal(t, root.Id, organization.OwnerUserId)
	var fund model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&fund).Error)
	var memberFund model.OrganizationMemberFund
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", organization.Id, root.Id).First(&memberFund).Error)
	var ledger model.OrganizationQuotaLedger
	require.NoError(t, db.Where("idempotency_key = ?", "setup:"+strconv.Itoa(root.Id)+":root-grant").First(&ledger).Error)
	assert.Equal(t, int64(100000000), ledger.UserQuotaDelta)
}
