package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	MidjourneyStatusSubmitting = "SUBMITTING"

	MidjourneyBillingStatusReserved = "reserved"
	MidjourneyBillingStatusSettled  = "settled"
	MidjourneyBillingStatusRefunded = "refunded"
)

var ErrMidjourneyRedisTokenReservationUnsupported = errors.New("limited token Midjourney billing is unavailable while Redis quota reservations are enabled")

type Midjourney struct {
	Id             int    `json:"id"`
	Code           int    `json:"code"`
	UserId         int    `json:"user_id" gorm:"index"`
	OrganizationId int    `json:"organization_id" gorm:"index"`
	ProjectId      *int   `json:"project_id,omitempty" gorm:"index"`
	Action         string `json:"action" gorm:"type:varchar(40);index"`
	MjId           string `json:"mj_id" gorm:"index"`
	Prompt         string `json:"prompt"`
	PromptEn       string `json:"prompt_en"`
	Description    string `json:"description"`
	State          string `json:"state"`
	SubmitTime     int64  `json:"submit_time" gorm:"index"`
	StartTime      int64  `json:"start_time" gorm:"index"`
	FinishTime     int64  `json:"finish_time" gorm:"index"`
	ImageUrl       string `json:"image_url"`
	VideoUrl       string `json:"video_url"`
	VideoUrls      string `json:"video_urls"`
	Status         string `json:"status" gorm:"type:varchar(20);index"`
	Progress       string `json:"progress" gorm:"type:varchar(30);index"`
	FailReason     string `json:"fail_reason"`
	ChannelId      int    `json:"channel_id"`
	Quota          int    `json:"quota"`
	Buttons        string `json:"buttons"`
	Properties     string `json:"properties"`

	TokenId                   int    `json:"-" gorm:"default:0"`
	BillingTokenId            int    `json:"-" gorm:"default:0"`
	BillingTokenReserved      bool   `json:"-"`
	BillingChannelId          int    `json:"-" gorm:"default:0"`
	OrganizationReservationId int64  `json:"-" gorm:"default:0"`
	BillingStatus             string `json:"-" gorm:"type:varchar(20);index"`
	UpstreamAccepted          bool   `json:"-"`
	// LegacyOrganizationWallet is set only by the organization migration for
	// tasks whose wallet charge predates durable organization reservations.
	LegacyOrganizationWallet bool               `json:"-"`
	QuotaClamp               *common.QuotaClamp `json:"-" gorm:"-"`
}

// BeforeCreate snapshots the authoritative organization from the owning user.
func (m *Midjourney) BeforeCreate(tx *gorm.DB) error {
	return overwriteOrganizationSnapshot(tx, m.UserId, &m.OrganizationId)
}

// TaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type TaskQueryParams struct {
	ChannelID      string
	MjID           string
	StartTimestamp string
	EndTimestamp   string
}

func GetAllUserTask(userId int, startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllTasks(startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllUnFinishTasks() []*Midjourney {
	var tasks []*Midjourney
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").
		Where("mj_id <> ?", "").
		Where("billing_status IS NULL OR billing_status <> ?", MidjourneyBillingStatusReserved).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedMidjourneyTasks reports whether at least one Midjourney task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the midjourney_poll system task needs to run; when no task is pending
// the scheduler skips creating a row entirely.
func HasUnfinishedMidjourneyTasks() bool {
	var id int
	err := DB.Model(&Midjourney{}).
		Where("progress != ? OR billing_status = ?", "100%", MidjourneyBillingStatusReserved).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByOnlyMJId(mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJId(userId int, mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id = ?", userId, mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJIds(userId int, mjIds []string) []*Midjourney {
	var mj []*Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id in (?)", userId, mjIds).Find(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetMjByuId(id int) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("id = ?", id).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func UpdateProgress(id int, progress string) error {
	return DB.Model(&Midjourney{}).Where("id = ?", id).Update("progress", progress).Error
}

func (midjourney *Midjourney) Insert() error {
	organizationId, err := organizationIDForUser(DB, midjourney.UserId)
	if err != nil {
		return err
	}
	return midjourney.InsertPreparedBilling(organizationId)
}

func (midjourney *Midjourney) Update() error {
	var err error
	err = DB.Save(midjourney).Error
	return err
}

func (midjourney *Midjourney) UpdateBillingState() error {
	return DB.Model(midjourney).
		Select("quota", "token_id", "billing_token_id", "billing_token_reserved", "billing_channel_id", "organization_reservation_id", "billing_status").
		Updates(midjourney).Error
}

// InsertPreparedBilling persists the local submission before the upstream call.
// For organization members the wallet reservation and its task binding commit in
// the same transaction, so a process crash cannot leave an unbound debit.
func (midjourney *Midjourney) InsertPreparedBilling(expectedOrganizationId int) error {
	if DB == nil || midjourney == nil || midjourney.UserId <= 0 || expectedOrganizationId < 0 ||
		midjourney.Quota < 0 || midjourney.Quota >= common.MaxQuota || midjourney.BillingTokenId < 0 ||
		midjourney.BillingChannelId < 0 || midjourney.TokenId != 0 || midjourney.OrganizationReservationId != 0 {
		return ErrOrganizationAccountingInvalid
	}

	reserved := false
	tokenReserved := false
	tokenKey := ""
	var quotaClamp *common.QuotaClamp
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(midjourney).Error; err != nil {
			return err
		}
		if midjourney.OrganizationId != expectedOrganizationId {
			return ErrOrganizationIdentityInvalid
		}
		if midjourney.OrganizationId == 0 || midjourney.Quota == 0 {
			return nil
		}

		requestId := fmt.Sprintf("midjourney:%d", midjourney.Id)
		reservation, err := ReserveOrganizationWalletQuotaTx(tx, OrganizationWalletReserveParams{
			OrganizationId: midjourney.OrganizationId,
			UserId:         midjourney.UserId,
			Amount:         int64(midjourney.Quota),
			RequestId:      requestId,
			IdempotencyKey: requestId + ":reserve",
			SourceType:     "midjourney",
			SourceId:       strconv.Itoa(midjourney.Id),
			Actor: OrganizationAccountingActor{
				Kind:   OrganizationAccountingActorSystem,
				Policy: "midjourney_billing",
			},
		})
		if err != nil {
			return err
		}
		if midjourney.BillingTokenId > 0 {
			var token Token
			if err := lockForUpdate(tx).Where("id = ?", midjourney.BillingTokenId).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != midjourney.UserId || token.OrganizationId != midjourney.OrganizationId {
				return ErrOrganizationIdentityInvalid
			}
			if common.RedisEnabled && !token.UnlimitedQuota {
				// Redis is authoritative for limited-token admission. A database-only
				// conditional update can race a normal Redis-first request and admit
				// more quota than the cached balance. Fail before the upstream call
				// until this path has a durable cross-store reservation protocol.
				return ErrMidjourneyRedisTokenReservationUnsupported
			}
			tokenQuery := tx.Model(&Token{}).
				Where("id = ? AND user_id = ? AND organization_id = ?", token.Id, midjourney.UserId, midjourney.OrganizationId)
			if !token.UnlimitedQuota {
				tokenQuery = tokenQuery.Where("remain_quota >= ?", midjourney.Quota)
			}
			tokenUsedQuota, clamp := common.SaturatingInt32CounterAddChecked(token.UsedQuota, midjourney.Quota)
			if quotaClamp == nil {
				quotaClamp = clamp
			}
			tokenUpdate := tokenQuery.Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota - ?", midjourney.Quota),
				"used_quota":    tokenUsedQuota,
				"accessed_time": common.GetTimestamp(),
			})
			if tokenUpdate.Error != nil {
				return tokenUpdate.Error
			}
			if tokenUpdate.RowsAffected != 1 {
				return fmt.Errorf("token quota is not enough")
			}
			tokenReserved = true
			tokenKey = token.Key
		}
		update := tx.Model(&Midjourney{}).
			Where("id = ? AND organization_id = ? AND organization_reservation_id = 0", midjourney.Id, midjourney.OrganizationId).
			Updates(map[string]interface{}{
				"organization_reservation_id": reservation.Reservation.Id,
				"billing_token_reserved":      tokenReserved,
				"billing_status":              MidjourneyBillingStatusReserved,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrOrganizationReservationState
		}
		midjourney.OrganizationReservationId = reservation.Reservation.Id
		midjourney.BillingTokenReserved = tokenReserved
		midjourney.BillingStatus = MidjourneyBillingStatusReserved
		reserved = !reservation.Accounting.AlreadyApplied
		return nil
	})
	if err != nil {
		return err
	}
	midjourney.QuotaClamp = quotaClamp
	if reserved {
		syncOrganizationAccountingQuotaCache(midjourney.UserId, -int64(midjourney.Quota), OrganizationLedgerReserve)
	}
	if tokenReserved && tokenKey != "" && common.RedisEnabled {
		if _, err := cacheApplyTokenQuotaDelta(midjourney.BillingTokenId, tokenKey, int64(-midjourney.Quota)); err != nil {
			common.SysLog("failed to sync Midjourney token reservation cache: " + err.Error())
		}
	}
	return nil
}

// UpdateSubmissionResult saves the upstream identity and visible task state
// without touching any billing or tenant ownership fields.
func (midjourney *Midjourney) UpdateSubmissionResult() error {
	if DB == nil || midjourney == nil || midjourney.Id <= 0 {
		return ErrOrganizationAccountingInvalid
	}
	result := DB.Model(&Midjourney{}).Where("id = ? AND user_id = ?", midjourney.Id, midjourney.UserId).
		Updates(map[string]interface{}{
			"code":              midjourney.Code,
			"mj_id":             midjourney.MjId,
			"description":       midjourney.Description,
			"prompt_en":         midjourney.PromptEn,
			"state":             midjourney.State,
			"start_time":        midjourney.StartTime,
			"finish_time":       midjourney.FinishTime,
			"image_url":         midjourney.ImageUrl,
			"video_url":         midjourney.VideoUrl,
			"video_urls":        midjourney.VideoUrls,
			"status":            midjourney.Status,
			"progress":          midjourney.Progress,
			"fail_reason":       midjourney.FailReason,
			"buttons":           midjourney.Buttons,
			"properties":        midjourney.Properties,
			"upstream_accepted": midjourney.UpstreamAccepted,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	if result.RowsAffected != 0 {
		return gorm.ErrRecordNotFound
	}

	// MySQL with clientFoundRows disabled reports zero changed rows for a
	// successful no-op update. Re-read and compare every submitted field so the
	// no-op is accepted without hiding a missing row or a concurrent overwrite.
	var persisted Midjourney
	if err := DB.Select(
		"code", "mj_id", "description", "prompt_en", "state", "start_time", "finish_time",
		"image_url", "video_url", "video_urls", "status", "progress", "fail_reason",
		"buttons", "properties", "upstream_accepted",
	).Where("id = ? AND user_id = ?", midjourney.Id, midjourney.UserId).First(&persisted).Error; err != nil {
		return err
	}
	if persisted.Code != midjourney.Code || persisted.MjId != midjourney.MjId ||
		persisted.Description != midjourney.Description || persisted.PromptEn != midjourney.PromptEn ||
		persisted.State != midjourney.State || persisted.StartTime != midjourney.StartTime ||
		persisted.FinishTime != midjourney.FinishTime || persisted.ImageUrl != midjourney.ImageUrl ||
		persisted.VideoUrl != midjourney.VideoUrl || persisted.VideoUrls != midjourney.VideoUrls ||
		persisted.Status != midjourney.Status || persisted.Progress != midjourney.Progress ||
		persisted.FailReason != midjourney.FailReason || persisted.Buttons != midjourney.Buttons ||
		persisted.Properties != midjourney.Properties || persisted.UpstreamAccepted != midjourney.UpstreamAccepted {
		return ErrOrganizationReservationState
	}
	return nil
}

type MidjourneyOrganizationBillingResult struct {
	Applied    bool
	Settled    bool
	QuotaClamp *common.QuotaClamp
}

func truncateBillingLogText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func buildMidjourneyBillingLogOutboxPayloadTx(
	tx *gorm.DB,
	task *Midjourney,
	logType int,
	quota int,
	content string,
	requestId string,
	reservation BillingLogReservationSnapshot,
	ledger BillingLogLedgerSnapshot,
	other map[string]interface{},
) (BillingLogOutboxPayload, error) {
	if tx == nil || task == nil {
		return BillingLogOutboxPayload{}, ErrBillingLogOutboxInvalid
	}

	var organization Organization
	if err := tx.Unscoped().Select("id", "name").Where("id = ?", task.OrganizationId).Limit(1).Find(&organization).Error; err != nil {
		return BillingLogOutboxPayload{}, err
	}
	var user User
	if err := tx.Unscoped().Select("id", "username").Where("id = ?", task.UserId).Limit(1).Find(&user).Error; err != nil {
		return BillingLogOutboxPayload{}, err
	}

	tokenId := task.BillingTokenId
	if tokenId <= 0 {
		tokenId = task.TokenId
	}
	var token Token
	if tokenId > 0 {
		if err := tx.Unscoped().Select("id", "name").Where("id = ?", tokenId).Limit(1).Find(&token).Error; err != nil {
			return BillingLogOutboxPayload{}, err
		}
	}
	channelId := task.GetBillingChannelId()
	var channel Channel
	if channelId > 0 {
		if err := tx.Unscoped().Select("id", "name").Where("id = ?", channelId).Limit(1).Find(&channel).Error; err != nil {
			return BillingLogOutboxPayload{}, err
		}
	}

	logOther := make(map[string]interface{}, len(other)+3)
	for key, value := range other {
		logOther[key] = value
	}
	logOther["task_id"] = truncateBillingLogText(task.MjId, 1024)
	logOther["local_task_id"] = task.Id
	logOther["action"] = task.Action
	if task.QuotaClamp != nil {
		logOther["admin_info"] = map[string]interface{}{
			"quota_saturation": task.QuotaClamp.AuditMap(),
		}
	}

	payload := BillingLogOutboxPayload{
		Version: BillingLogOutboxPayloadVersion,
		LogType: logType,
		Organization: BillingLogResourceSnapshot{
			Id:   task.OrganizationId,
			Name: truncateBillingLogText(organization.Name, 128),
		},
		User: BillingLogResourceSnapshot{
			Id:   task.UserId,
			Name: truncateBillingLogText(user.Username, 128),
		},
		Token: BillingLogResourceSnapshot{
			Id:   tokenId,
			Name: truncateBillingLogText(token.Name, 128),
		},
		Channel: BillingLogResourceSnapshot{
			Id:   channelId,
			Name: truncateBillingLogText(channel.Name, 128),
		},
		Model:       "mj_" + strings.ToLower(task.Action),
		Quota:       quota,
		Reason:      content,
		Reservation: reservation,
		Ledger:      ledger,
		RequestId:   requestId,
		CreatedAt:   common.GetTimestamp(),
		Other:       logOther,
	}
	if task.ProjectId != nil && *task.ProjectId > 0 {
		payload.Project = &BillingLogResourceSnapshot{Id: *task.ProjectId}
	}
	return payload, nil
}

// SettleOrganizationBilling atomically settles the reservation, token marker,
// aggregate usage and durable task billing state on the main database.
func (midjourney *Midjourney) SettleOrganizationBilling() (MidjourneyOrganizationBillingResult, error) {
	if DB == nil || midjourney == nil || midjourney.Id <= 0 {
		return MidjourneyOrganizationBillingResult{}, ErrOrganizationAccountingInvalid
	}

	result := MidjourneyOrganizationBillingResult{QuotaClamp: midjourney.QuotaClamp}
	userWalletDelta := int64(0)
	walletProjectionNeeded := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Midjourney
		if err := lockForUpdate(tx).Where("id = ?", midjourney.Id).First(&task).Error; err != nil {
			return err
		}
		if task.UserId <= 0 || task.OrganizationId <= 0 || task.OrganizationReservationId <= 0 ||
			task.Quota <= 0 || task.Quota >= common.MaxQuota || task.BillingTokenId < 0 ||
			task.BillingChannelId <= 0 || strings.TrimSpace(task.MjId) == "" || !task.UpstreamAccepted {
			return ErrOrganizationAccountingInvalid
		}
		if task.BillingStatus == MidjourneyBillingStatusSettled {
			*midjourney = task
			result.Settled = true
			return nil
		}
		if task.BillingStatus != MidjourneyBillingStatusReserved {
			return ErrOrganizationReservationState
		}

		requestId := fmt.Sprintf("midjourney:%d", task.Id)
		settled, delta, err := settleOrganizationWalletQuotaTx(tx, OrganizationWalletSettleParams{
			ReservationId:  task.OrganizationReservationId,
			ActualQuota:    int64(task.Quota),
			IdempotencyKey: requestId + ":settle",
			RequestId:      requestId,
			Actor: OrganizationAccountingActor{
				Kind:   OrganizationAccountingActorSystem,
				Policy: "midjourney_billing",
			},
		})
		if err != nil {
			return err
		}
		if settled.Reservation.OrganizationId != task.OrganizationId || settled.Reservation.UserId != task.UserId ||
			settled.Reservation.SettledQuota != int64(task.Quota) {
			return ErrOrganizationIdentityInvalid
		}
		userWalletDelta = delta
		walletProjectionNeeded = !settled.Accounting.AlreadyApplied

		if task.BillingTokenReserved != (task.BillingTokenId > 0) {
			return ErrOrganizationAccountingInvalid
		}
		if task.BillingTokenId > 0 {
			var token Token
			if err := lockForUpdate(tx.Unscoped()).Where("id = ?", task.BillingTokenId).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != task.UserId || token.OrganizationId != task.OrganizationId {
				return ErrOrganizationIdentityInvalid
			}
		}

		var user User
		if err := lockForUpdate(tx).
			Select("id", "organization_id", "used_quota", "request_count").
			Where("id = ?", task.UserId).
			First(&user).Error; err != nil {
			return err
		}
		if user.OrganizationId != task.OrganizationId {
			return ErrOrganizationIdentityInvalid
		}
		usedQuota, usedQuotaClamp := common.SaturatingInt32CounterAddChecked(user.UsedQuota, task.Quota)
		requestCount, requestCountClamp := common.SaturatingInt32CounterAddChecked(user.RequestCount, 1)
		if result.QuotaClamp == nil {
			result.QuotaClamp = usedQuotaClamp
		}
		if result.QuotaClamp == nil {
			result.QuotaClamp = requestCountClamp
		}
		userUsage := tx.Model(&User{}).Where("id = ? AND organization_id = ?", task.UserId, task.OrganizationId).
			Updates(map[string]interface{}{
				"used_quota":    usedQuota,
				"request_count": requestCount,
			})
		if userUsage.Error != nil {
			return userUsage.Error
		}
		channelUsage := tx.Model(&Channel{}).Where("id = ?", task.BillingChannelId).
			Update("used_quota", gorm.Expr("used_quota + ?", task.Quota))
		if channelUsage.Error != nil {
			return channelUsage.Error
		}

		taskUpdate := tx.Model(&Midjourney{}).
			Where("id = ? AND billing_status = ? AND organization_reservation_id = ?", task.Id, MidjourneyBillingStatusReserved, task.OrganizationReservationId).
			Updates(map[string]interface{}{
				"token_id":       task.BillingTokenId,
				"billing_status": MidjourneyBillingStatusSettled,
			})
		if taskUpdate.Error != nil {
			return taskUpdate.Error
		}
		if taskUpdate.RowsAffected != 1 {
			return ErrOrganizationReservationState
		}

		task.TokenId = task.BillingTokenId
		task.BillingStatus = MidjourneyBillingStatusSettled
		task.QuotaClamp = result.QuotaClamp
		if common.LogConsumeEnabled {
			payload, err := buildMidjourneyBillingLogOutboxPayloadTx(
				tx,
				&task,
				LogTypeConsume,
				task.Quota,
				"Midjourney task consumption",
				requestId,
				BillingLogReservationSnapshot{
					Id:            settled.Reservation.Id,
					Status:        settled.Reservation.Status,
					ReservedQuota: int(settled.Reservation.ReservedQuota),
					SettledQuota:  int(settled.Reservation.SettledQuota),
				},
				BillingLogLedgerSnapshot{Id: settled.Accounting.LedgerId, Operation: OrganizationLedgerSettle},
				nil,
			)
			if err != nil {
				return err
			}
			if _, err := EnqueueBillingLogOutboxTx(tx, fmt.Sprintf("midjourney:%d:consume", task.Id), payload); err != nil {
				return err
			}
		}
		*midjourney = task
		result.Applied = true
		result.Settled = true
		return nil
	})
	if err != nil {
		return MidjourneyOrganizationBillingResult{}, err
	}
	if result.Applied && walletProjectionNeeded && userWalletDelta != 0 {
		syncOrganizationAccountingQuotaCache(midjourney.UserId, userWalletDelta, OrganizationLedgerSettle)
	}
	return result, nil
}

// RefundOrganizationBilling atomically refunds either a prepared reservation or
// a settled task. Token and aggregate usage are reversed only for settled work.
func (midjourney *Midjourney) RefundOrganizationBilling(reason string, markFailure bool) (bool, error) {
	return midjourney.refundOrganizationBilling(reason, markFailure, nil)
}

// FailAndRefundOrganizationBilling couples the terminal status CAS with every
// refund side effect. A stale failure poll therefore cannot refund or overwrite
// a task that another poller has already moved to success.
func (midjourney *Midjourney) FailAndRefundOrganizationBilling(expectedStatus string, reason string) (bool, error) {
	return midjourney.refundOrganizationBilling(reason, true, &expectedStatus)
}

func (midjourney *Midjourney) refundOrganizationBilling(reason string, markFailure bool, expectedStatus *string) (bool, error) {
	if DB == nil || midjourney == nil || midjourney.Id <= 0 {
		return false, ErrOrganizationAccountingInvalid
	}

	desiredTaskState := *midjourney
	applied := false
	tokenKey := ""
	tokenRefundId := 0
	tokenRefundDelta := int64(0)
	walletDelta := int64(0)
	walletProjectionNeeded := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Midjourney
		if err := lockForUpdate(tx).Where("id = ?", midjourney.Id).First(&task).Error; err != nil {
			return err
		}
		if task.UserId <= 0 || task.OrganizationId <= 0 {
			return ErrOrganizationIdentityInvalid
		}
		if expectedStatus != nil && task.Status != *expectedStatus {
			*midjourney = task
			return nil
		}
		if task.Quota == 0 || task.BillingStatus == MidjourneyBillingStatusRefunded {
			*midjourney = task
			return nil
		}
		if task.Quota < 0 || task.Quota >= common.MaxQuota {
			return ErrOrganizationAccountingInvalid
		}

		wasSettled := task.BillingStatus == MidjourneyBillingStatusSettled
		logReservation := BillingLogReservationSnapshot{}
		logLedger := BillingLogLedgerSnapshot{}
		requestId := fmt.Sprintf("midjourney:%d:refund", task.Id)
		if task.OrganizationReservationId > 0 {
			var reservation OrganizationWalletReservation
			if err := lockForUpdate(tx).Where("id = ?", task.OrganizationReservationId).First(&reservation).Error; err != nil {
				return err
			}
			if reservation.OrganizationId != task.OrganizationId || reservation.UserId != task.UserId || reservation.ReservedQuota != int64(task.Quota) {
				return ErrOrganizationIdentityInvalid
			}
			if task.BillingStatus == "" {
				wasSettled = reservation.Status == OrganizationWalletReservationSettled || task.TokenId > 0
			}
			refunded, delta, err := refundOrganizationWalletQuotaTx(tx, OrganizationWalletRefundParams{
				ReservationId:  task.OrganizationReservationId,
				IdempotencyKey: requestId,
				RequestId:      requestId,
				Actor: OrganizationAccountingActor{
					Kind:   OrganizationAccountingActorSystem,
					Policy: "midjourney_billing",
				},
			})
			if err != nil {
				return err
			}
			walletDelta = delta
			walletProjectionNeeded = !refunded.Accounting.AlreadyApplied
			logReservation = BillingLogReservationSnapshot{
				Id:            refunded.Reservation.Id,
				Status:        refunded.Reservation.Status,
				ReservedQuota: int(refunded.Reservation.ReservedQuota),
				SettledQuota:  int(refunded.Reservation.SettledQuota),
			}
			logLedger = BillingLogLedgerSnapshot{Id: refunded.Accounting.LedgerId, Operation: OrganizationLedgerRefund}
		} else if task.LegacyOrganizationWallet {
			legacyRequestId := fmt.Sprintf("midjourney:%d:legacy-refund", task.Id)
			credited, err := CreditOrganizationUserWalletTx(tx, OrganizationWalletCreditParams{
				OrganizationId: task.OrganizationId,
				UserId:         task.UserId,
				Amount:         int64(task.Quota),
				SourceType:     "wallet_refund",
				SourceId:       legacyRequestId,
				IdempotencyKey: legacyRequestId,
				RequestId:      legacyRequestId,
				Actor: OrganizationAccountingActor{
					Kind:   OrganizationAccountingActorSystem,
					Policy: "legacy_midjourney_billing",
				},
			})
			if err != nil {
				return err
			}
			wasSettled = true
			walletDelta = int64(task.Quota)
			walletProjectionNeeded = !credited.AlreadyApplied
			logLedger = BillingLogLedgerSnapshot{Id: credited.LedgerId, Operation: OrganizationLedgerRefund}
		} else {
			return ErrOrganizationLedgerRequired
		}

		shouldRefundToken := task.BillingTokenReserved && task.BillingTokenId > 0
		if task.BillingStatus == "" && wasSettled && task.TokenId > 0 {
			shouldRefundToken = true
			task.BillingTokenId = task.TokenId
		}
		if shouldRefundToken {
			var token Token
			err := lockForUpdate(tx.Unscoped()).Where("id = ?", task.BillingTokenId).First(&token).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				if token.UserId != task.UserId || token.OrganizationId != task.OrganizationId {
					return ErrOrganizationIdentityInvalid
				}
				tokenUsedQuota, clamp := common.SaturatingInt32CounterAddChecked(token.UsedQuota, -task.Quota)
				if task.QuotaClamp == nil {
					task.QuotaClamp = clamp
				}
				tokenUpdate := tx.Unscoped().Model(&Token{}).
					Where("id = ? AND user_id = ? AND organization_id = ?", token.Id, task.UserId, task.OrganizationId).
					Updates(map[string]interface{}{
						"remain_quota":  gorm.Expr("remain_quota + ?", task.Quota),
						"used_quota":    tokenUsedQuota,
						"accessed_time": common.GetTimestamp(),
					})
				if tokenUpdate.Error != nil {
					return tokenUpdate.Error
				}
				if tokenUpdate.RowsAffected != 1 {
					return gorm.ErrRecordNotFound
				}
				tokenKey = token.Key
				tokenRefundId = token.Id
				tokenRefundDelta = int64(task.Quota)
			}
		}
		if wasSettled {
			var user User
			if err := lockForUpdate(tx).
				Select("id", "organization_id", "used_quota").
				Where("id = ?", task.UserId).
				First(&user).Error; err != nil {
				return err
			}
			if user.OrganizationId != task.OrganizationId {
				return ErrOrganizationIdentityInvalid
			}
			usedQuota, clamp := common.SaturatingInt32CounterAddChecked(user.UsedQuota, -task.Quota)
			if task.QuotaClamp == nil {
				task.QuotaClamp = clamp
			}
			userUsage := tx.Model(&User{}).Where("id = ? AND organization_id = ?", task.UserId, task.OrganizationId).
				Update("used_quota", usedQuota)
			if userUsage.Error != nil {
				return userUsage.Error
			}
			channelId := task.GetBillingChannelId()
			if channelId > 0 {
				channelUsage := tx.Model(&Channel{}).Where("id = ?", channelId).
					Update("used_quota", gorm.Expr("used_quota - ?", task.Quota))
				if channelUsage.Error != nil {
					return channelUsage.Error
				}
			}
		}

		updates := map[string]interface{}{
			"quota":                  0,
			"billing_token_reserved": false,
			"billing_status":         MidjourneyBillingStatusRefunded,
		}
		if markFailure {
			updates["code"] = desiredTaskState.Code
			updates["prompt_en"] = desiredTaskState.PromptEn
			updates["state"] = desiredTaskState.State
			updates["start_time"] = desiredTaskState.StartTime
			updates["finish_time"] = desiredTaskState.FinishTime
			updates["image_url"] = desiredTaskState.ImageUrl
			updates["video_url"] = desiredTaskState.VideoUrl
			updates["video_urls"] = desiredTaskState.VideoUrls
			updates["buttons"] = desiredTaskState.Buttons
			updates["properties"] = desiredTaskState.Properties
			updates["status"] = "FAILURE"
			updates["progress"] = "100%"
			updates["fail_reason"] = reason
		}
		taskQuery := tx.Model(&Midjourney{}).
			Where("id = ? AND organization_id = ? AND quota = ?", task.Id, task.OrganizationId, task.Quota)
		if expectedStatus != nil {
			taskQuery = taskQuery.Where("status = ?", *expectedStatus)
		}
		taskUpdate := taskQuery.Updates(updates)
		if taskUpdate.Error != nil {
			return taskUpdate.Error
		}
		if taskUpdate.RowsAffected != 1 {
			return ErrOrganizationReservationState
		}
		if wasSettled {
			payload, err := buildMidjourneyBillingLogOutboxPayloadTx(
				tx,
				&task,
				LogTypeRefund,
				task.Quota,
				"Midjourney task quota refunded",
				requestId,
				logReservation,
				logLedger,
				map[string]interface{}{"reason": truncateBillingLogText(reason, 4096)},
			)
			if err != nil {
				return err
			}
			if _, err := EnqueueBillingLogOutboxTx(tx, fmt.Sprintf("midjourney:%d:refund", task.Id), payload); err != nil {
				return err
			}
		}

		task.Quota = 0
		task.BillingTokenReserved = false
		task.BillingStatus = MidjourneyBillingStatusRefunded
		if markFailure {
			task.Status = "FAILURE"
			task.Progress = "100%"
			task.FailReason = reason
		}
		*midjourney = task
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if applied && walletProjectionNeeded {
		syncOrganizationAccountingQuotaCache(midjourney.UserId, walletDelta, OrganizationLedgerRefund)
	}
	if applied && tokenKey != "" && common.RedisEnabled {
		if _, err := cacheApplyTokenQuotaDelta(tokenRefundId, tokenKey, tokenRefundDelta); err != nil {
			common.SysLog("failed to sync Midjourney token refund cache: " + err.Error())
		}
	}
	return applied, nil
}

func GetRecoverableMidjourneySubmissions(cutoffMilliseconds int64, limit int) ([]*Midjourney, error) {
	if cutoffMilliseconds <= 0 || limit <= 0 {
		return nil, ErrOrganizationAccountingInvalid
	}
	var tasks []*Midjourney
	err := DB.Where("submit_time > 0 AND submit_time < ?", cutoffMilliseconds).
		Where("billing_status = ? OR (status = ? AND mj_id = ?)", MidjourneyBillingStatusReserved, MidjourneyStatusSubmitting, "").
		Order("id").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func (midjourney *Midjourney) FailStaleSubmission(reason string) (bool, error) {
	if midjourney == nil || midjourney.Id <= 0 || strings.TrimSpace(reason) == "" {
		return false, ErrOrganizationAccountingInvalid
	}
	result := DB.Model(&Midjourney{}).
		Where("id = ? AND status = ? AND mj_id = ? AND quota = 0", midjourney.Id, MidjourneyStatusSubmitting, "").
		Updates(map[string]interface{}{
			"status":      "FAILURE",
			"progress":    "100%",
			"fail_reason": reason,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// SettleTokenQuota atomically applies token accounting and its durable refund
// marker. Locking the task before the token gives settlement retries one stable
// lock order and lets an already-persisted marker make the operation idempotent.
func (midjourney *Midjourney) SettleTokenQuota(tokenId int, tokenKey string, quota int) (bool, error) {
	if midjourney == nil || midjourney.Id <= 0 || tokenId <= 0 || quota < 0 {
		return false, fmt.Errorf("invalid Midjourney token settlement")
	}

	applied := false
	var quotaClamp *common.QuotaClamp
	err := DB.Transaction(func(tx *gorm.DB) error {
		var persisted Midjourney
		if err := lockForUpdate(tx).
			Select("id", "quota", "token_id").
			Where("id = ?", midjourney.Id).
			First(&persisted).Error; err != nil {
			return err
		}
		if persisted.TokenId != 0 {
			if persisted.TokenId != tokenId {
				return fmt.Errorf("Midjourney task token marker mismatch")
			}
			midjourney.TokenId = persisted.TokenId
			return nil
		}
		if persisted.Quota != quota {
			return fmt.Errorf("Midjourney task quota changed before token settlement")
		}

		var token Token
		if err := lockForUpdate(tx).Select("id", "used_quota").Where("id = ?", tokenId).First(&token).Error; err != nil {
			return err
		}
		if quota > 0 {
			tokenUsedQuota, clamp := common.SaturatingInt32CounterAddChecked(token.UsedQuota, quota)
			result := tx.Model(&Token{}).Where("id = ?", tokenId).Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota - ?", quota),
				"used_quota":    tokenUsedQuota,
				"accessed_time": common.GetTimestamp(),
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			quotaClamp = clamp
		}

		result := tx.Model(&Midjourney{}).
			Where("id = ? AND token_id = 0 AND quota = ?", midjourney.Id, quota).
			Update("token_id", tokenId)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("Midjourney token marker was updated concurrently")
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}

	midjourney.TokenId = tokenId
	if midjourney.QuotaClamp == nil {
		midjourney.QuotaClamp = quotaClamp
	}
	if applied && quota > 0 && common.RedisEnabled {
		if _, err := cacheApplyTokenQuotaDelta(tokenId, tokenKey, int64(-quota)); err != nil {
			common.SysLog("failed to settle Midjourney token quota in cache: " + err.Error())
		}
	}
	return applied, nil
}

// BindOrganizationReservation durably marks the one caller that may apply
// token billing after the organization wallet reservation has settled.
func (midjourney *Midjourney) BindOrganizationReservation(reservationId int64) (bool, error) {
	if midjourney == nil || midjourney.Id <= 0 || reservationId <= 0 {
		return false, ErrOrganizationAccountingInvalid
	}
	result := DB.Model(&Midjourney{}).
		Where("id = ? AND organization_reservation_id = 0", midjourney.Id).
		Update("organization_reservation_id", reservationId)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		midjourney.OrganizationReservationId = reservationId
		return true, nil
	}
	var persistedReservationId int64
	if err := DB.Model(&Midjourney{}).Select("organization_reservation_id").Where("id = ?", midjourney.Id).Scan(&persistedReservationId).Error; err != nil {
		return false, err
	}
	if persistedReservationId != reservationId {
		return false, ErrOrganizationReservationState
	}
	midjourney.OrganizationReservationId = persistedReservationId
	return false, nil
}

// ClearOrganizationBillingQuota marks a successful organization refund. Only
// the caller that wins this CAS may refund token and aggregate usage counters.
func (midjourney *Midjourney) ClearOrganizationBillingQuota(expectedQuota int) (bool, error) {
	if midjourney == nil || midjourney.Id <= 0 || expectedQuota <= 0 {
		return false, ErrOrganizationAccountingInvalid
	}
	query := DB.Model(&Midjourney{}).Where("id = ? AND quota = ?", midjourney.Id, expectedQuota)
	if midjourney.OrganizationReservationId > 0 {
		query = query.Where("organization_reservation_id = ?", midjourney.OrganizationReservationId)
	} else if midjourney.LegacyOrganizationWallet {
		query = query.Where("organization_reservation_id = 0 AND legacy_organization_wallet = ?", true)
	} else {
		return false, ErrOrganizationAccountingInvalid
	}
	result := query.Update("quota", 0)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	midjourney.Quota = 0
	return true, nil
}

func (midjourney *Midjourney) GetBillingChannelId() int {
	if midjourney.BillingChannelId > 0 {
		return midjourney.BillingChannelId
	}
	return midjourney.ChannelId
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus.
// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Uses Model().Select("*").Updates() to avoid GORM Save()'s INSERT fallback.
func (midjourney *Midjourney) UpdateWithStatus(fromStatus string) (bool, error) {
	result := DB.Model(midjourney).Where("status = ?", fromStatus).Select("*").Updates(midjourney)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func MjBulkUpdate(mjIds []string, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("mj_id in (?)", mjIds).
		Updates(params).Error
}

func MjBulkUpdateByTaskIds(taskIDs []int, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// CountAllTasks returns total midjourney tasks for admin query
func CountAllTasks(queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// CountAllUserTask returns total midjourney tasks for user
func CountAllUserTask(userId int, queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{}).Where("user_id = ?", userId)
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
