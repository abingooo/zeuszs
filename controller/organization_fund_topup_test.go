package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createOrganizationTopUpControllerScope(t *testing.T, db *gorm.DB, role model.OrganizationRole) (model.Organization, model.User) {
	t.Helper()
	owner := organizationControllerUser(t, db, "topup-contract-owner", common.RoleCommonUser)
	actor := owner
	if role != model.OrganizationRoleOwner {
		actor = organizationControllerUser(t, db, "topup-contract-actor", common.RoleCommonUser)
	}
	organization := model.Organization{
		Name: "TopUp Contract Organization", Status: model.OrganizationStatusActive,
		OwnerUserId: owner.Id, AllowMemberTopup: true, PolicyVersion: 1,
	}
	require.NoError(t, db.Create(&organization).Error)
	require.NoError(t, db.Create(&model.OrganizationFundAccount{OrganizationId: organization.Id}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]interface{}{
		"organization_id": organization.Id, "organization_role": model.OrganizationRoleOwner,
		"organization_status": model.OrganizationMemberStatusActive,
	}).Error)
	if actor.Id != owner.Id {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", actor.Id).Updates(map[string]interface{}{
			"organization_id": organization.Id, "organization_role": role,
			"organization_status": model.OrganizationMemberStatusActive,
		}).Error)
	}
	actor.OrganizationId = organization.Id
	actor.OrganizationRole = role
	actor.OrganizationStatus = model.OrganizationMemberStatusActive
	return organization, actor
}

func TestTopUpInfoAdvertisesPersonalAndOrganizationTargets(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	organization, owner := createOrganizationTopUpControllerScope(t, db, model.OrganizationRoleOwner)
	confirmPaymentComplianceForTest(t)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", owner.Id)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"default_topup_target":"personal"`)
	assert.Contains(t, recorder.Body.String(), `"organization_fund_topup_enabled":true`)
	assert.Contains(t, recorder.Body.String(), `"organization":{"enabled":true,"organization_id":`)
	assert.Contains(t, recorder.Body.String(), `"organization_id":`+strconv.Itoa(organization.Id))
}

func TestOrganizationFundAmountRequestUsesExplicitTargetAndRejectsMember(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	previousDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	common.QuotaPerUnit = 10
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplayType
	})

	t.Run("owner capacity", func(t *testing.T) {
		db := setupOrganizationControllerTestDB(t)
		_, owner := createOrganizationTopUpControllerScope(t, db, model.OrganizationRoleOwner)
		require.NoError(t, validateTopUpTargetCapacity(owner.Id, model.TopUpTargetOrganization, 20))
	})

	t.Run("member", func(t *testing.T) {
		db := setupOrganizationControllerTestDB(t)
		_, member := createOrganizationTopUpControllerScope(t, db, model.OrganizationRoleMember)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", member.Id)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", strings.NewReader(`{"amount":2,"topup_target":"organization"}`))
		context.Request.Header.Set("Content-Type", "application/json")

		RequestAmount(context)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), model.ErrOrganizationAccountingForbidden.Error())
	})
}

func TestOrganizationTopUpRouteCannotOverrideForcedTarget(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(context, constant.ContextKeyTopUpTarget, model.TopUpTargetOrganization)

	target, err := resolveTopUpTarget(context, "")
	require.NoError(t, err)
	assert.Equal(t, model.TopUpTargetOrganization, target)
	_, err = resolveTopUpTarget(context, model.TopUpTargetPersonal)
	assert.ErrorIs(t, err, model.ErrInvalidTopUpTarget)
}

func TestAdminCompleteOrganizationTopUpUsesPersistedTargetAndIsIdempotent(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	organization, owner := createOrganizationTopUpControllerScope(t, db, model.OrganizationRoleOwner)
	actor := organizationControllerUser(t, db, "organization-topup-platform-admin", common.RoleAdminUser)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 10
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	order := model.TopUp{
		UserId: owner.Id, OrganizationId: organization.Id + 999,
		Amount: 2, Money: 2, TradeNo: "organization-admin-complete",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		TopUpTarget: model.TopUpTargetOrganization, CreateTime: common.GetTimestamp(),
		Status: common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&order).Error)
	assert.Equal(t, organization.Id, order.OrganizationId)

	requestBody := `{"trade_no":"organization-admin-complete","topup_target":"personal","organization_id":999999}`
	for range 2 {
		recorder := invokeOrganizationController(
			t,
			http.MethodPost,
			"/api/organization/admin/topup/complete",
			requestBody,
			AdminCompleteOrganizationTopUp,
			actor.Id,
		)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"success":true`)
	}

	var persisted model.TopUp
	require.NoError(t, db.First(&persisted, order.Id).Error)
	assert.Equal(t, model.TopUpTargetOrganization, persisted.TopUpTarget)
	assert.Equal(t, organization.Id, persisted.OrganizationId)
	assert.Equal(t, common.TopUpStatusSuccess, persisted.Status)
	var account model.OrganizationFundAccount
	require.NoError(t, db.Where("organization_id = ?", organization.Id).First(&account).Error)
	assert.EqualValues(t, 20, account.Quota)
	var refreshedOwner model.User
	require.NoError(t, db.First(&refreshedOwner, owner.Id).Error)
	assert.Zero(t, refreshedOwner.Quota)
	var ledgers int64
	require.NoError(t, db.Model(&model.OrganizationQuotaLedger{}).Where("source_id = ?", order.TradeNo).Count(&ledgers).Error)
	assert.EqualValues(t, 1, ledgers)
}

func TestAdminCompleteOrganizationTopUpRejectsPersonalOrder(t *testing.T) {
	db := setupOrganizationControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	_, owner := createOrganizationTopUpControllerScope(t, db, model.OrganizationRoleOwner)
	actor := organizationControllerUser(t, db, "personal-topup-platform-admin", common.RoleAdminUser)
	order := model.TopUp{
		UserId: owner.Id, Amount: 2, Money: 2, TradeNo: "personal-admin-complete",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		TopUpTarget: model.TopUpTargetPersonal, CreateTime: common.GetTimestamp(),
		Status: common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&order).Error)

	recorder := invokeOrganizationController(
		t,
		http.MethodPost,
		"/api/organization/admin/topup/complete",
		`{"trade_no":"personal-admin-complete"}`,
		AdminCompleteOrganizationTopUp,
		actor.Id,
	)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), model.ErrInvalidTopUpTarget.Error())

	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var ownerAfter model.User
	require.NoError(t, db.First(&ownerAfter, owner.Id).Error)
	assert.Zero(t, ownerAfter.Quota)
}
