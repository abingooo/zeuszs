package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	commonRelay "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

type TaskStatus string

func (t TaskStatus) ToVideoStatus() string {
	var status string
	switch t {
	case TaskStatusQueued, TaskStatusSubmitted:
		status = dto.VideoStatusQueued
	case TaskStatusInProgress:
		status = dto.VideoStatusInProgress
	case TaskStatusSuccess:
		status = dto.VideoStatusCompleted
	case TaskStatusFailure:
		status = dto.VideoStatusFailed
	default:
		status = dto.VideoStatusUnknown // Default fallback
	}
	return status
}

const (
	TaskStatusNotStart   TaskStatus = "NOT_START"
	TaskStatusSubmitted             = "SUBMITTED"
	TaskStatusQueued                = "QUEUED"
	TaskStatusInProgress            = "IN_PROGRESS"
	TaskStatusFailure               = "FAILURE"
	TaskStatusSuccess               = "SUCCESS"
	TaskStatusUnknown               = "UNKNOWN"
)

// TaskRefundLegacyCutoff separates tasks created before timeout refunds were
// introduced. Those legacy tasks are failed without an automatic refund.
const TaskRefundLegacyCutoff int64 = 1771718400 // 2026-02-22 00:00:00 UTC

type Task struct {
	ID              int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt       int64                 `json:"created_at" gorm:"index"`
	UpdatedAt       int64                 `json:"updated_at"`
	TaskID          string                `json:"task_id" gorm:"type:varchar(191);index"` // 第三方id，不一定有/ song id\ Task id
	Platform        constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"` // 平台
	UserId          int                   `json:"user_id" gorm:"index"`
	OrganizationId  int                   `json:"organization_id" gorm:"index"`
	ProjectId       *int                  `json:"project_id,omitempty" gorm:"index"`
	Group           string                `json:"group" gorm:"type:varchar(50)"` // 修正计费用
	ChannelId       int                   `json:"channel_id" gorm:"index"`
	Quota           int                   `json:"quota"`
	BillingRevision int64                 `json:"-" gorm:"type:bigint;not null;default:0"`
	// LegacyOrganizationWallet is set only by the organization migration for
	// tasks whose wallet charge predates durable organization reservations.
	LegacyOrganizationWallet bool       `json:"-"`
	Action                   string     `json:"action" gorm:"type:varchar(40);index"` // 任务类型, song, lyrics, description-mode
	Status                   TaskStatus `json:"status" gorm:"type:varchar(20);index"` // 任务状态
	FailReason               string     `json:"fail_reason"`
	SubmitTime               int64      `json:"submit_time" gorm:"index"`
	StartTime                int64      `json:"start_time" gorm:"index"`
	FinishTime               int64      `json:"finish_time" gorm:"index"`
	Progress                 string     `json:"progress" gorm:"type:varchar(20);index"`
	Properties               Properties `json:"properties" gorm:"type:json"`
	Username                 string     `json:"username,omitempty" gorm:"-"`
	// 禁止返回给用户，内部可能包含key等隐私信息
	PrivateData TaskPrivateData `json:"-" gorm:"column:private_data;type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
}

// BeforeCreate snapshots the authoritative organization from the owning user.
// A caller-supplied organization_id is deliberately overwritten.
func (t *Task) BeforeCreate(tx *gorm.DB) error {
	return overwriteOrganizationSnapshot(tx, t.UserId, &t.OrganizationId)
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = json.RawMessage(b)
}

func (t *Task) GetData(v any) error {
	return common.Unmarshal(t.Data, &v)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return common.Marshal(m)
}

type TaskPrivateData struct {
	Key            string `json:"key,omitempty"`
	UpstreamTaskID string `json:"upstream_task_id,omitempty"` // 上游真实 task ID
	ResultURL      string `json:"result_url,omitempty"`       // 任务成功后的结果 URL（视频地址等）
	// 计费上下文：用于异步退款/差额结算（轮询阶段读取）
	BillingSource                   string              `json:"billing_source,omitempty"`  // "wallet" 或 "subscription"
	SubscriptionId                  int                 `json:"subscription_id,omitempty"` // 订阅 ID，用于订阅退款
	OrganizationSubscriptionMetered bool                `json:"organization_subscription_metered,omitempty"`
	OrganizationReservationId       int64               `json:"organization_reservation_id,omitempty"`
	TokenId                         int                 `json:"token_id,omitempty"`        // 令牌 ID，用于令牌额度退款
	NodeName                        string              `json:"node_name,omitempty"`       // 发起任务的节点名，轮询结算阶段据此归属日志而非最后查询节点
	BillingContext                  *TaskBillingContext `json:"billing_context,omitempty"` // 计费参数快照（用于轮询阶段重新计算）
}

// TaskBillingContext 记录任务提交时的计费参数，以便轮询阶段可以重新计算额度。
type TaskBillingContext struct {
	ModelPrice      float64            `json:"model_price,omitempty"`       // 模型单价
	GroupRatio      float64            `json:"group_ratio,omitempty"`       // 分组倍率
	ModelRatio      float64            `json:"model_ratio,omitempty"`       // 模型倍率
	OtherRatios     map[string]float64 `json:"other_ratios,omitempty"`      // 附加倍率（时长、分辨率等）
	OriginModelName string             `json:"origin_model_name,omitempty"` // 模型名称，必须为OriginModelName
	PerCallBilling  bool               `json:"per_call_billing,omitempty"`  // 按次计费：跳过轮询阶段的差额结算
}

// GetUpstreamTaskID 获取上游真实 task ID（用于与 provider 通信）
// 旧数据没有 UpstreamTaskID 时，TaskID 本身就是上游 ID
func (t *Task) GetUpstreamTaskID() string {
	if t.PrivateData.UpstreamTaskID != "" {
		return t.PrivateData.UpstreamTaskID
	}
	return t.TaskID
}

// GetResultURL 获取任务结果 URL（视频地址等）
// 新数据存在 PrivateData.ResultURL 中；旧数据回退到 FailReason（历史兼容）
func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

// GenerateTaskID 生成对外暴露的 task_xxxx 格式 ID
func GenerateTaskID() string {
	key, _ := common.GenerateRandomCharsKey(32)
	return "task_" + key
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if (p == TaskPrivateData{}) {
		return nil, nil
	}
	return common.Marshal(p)
}

// SyncTaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
	UserIDs        []int
}

func InitTask(platform constant.TaskPlatform, relayInfo *commonRelay.RelayInfo) *Task {
	properties := Properties{}
	privateData := TaskPrivateData{}
	if relayInfo != nil && relayInfo.ChannelMeta != nil {
		if relayInfo.ChannelMeta.ChannelType == constant.ChannelTypeGemini ||
			relayInfo.ChannelMeta.ChannelType == constant.ChannelTypeVertexAi {
			privateData.Key = relayInfo.ChannelMeta.ApiKey
		}
		if relayInfo.UpstreamModelName != "" {
			properties.UpstreamModelName = relayInfo.UpstreamModelName
		}
		if relayInfo.OriginModelName != "" {
			properties.OriginModelName = relayInfo.OriginModelName
		}
	}

	// 使用预生成的公开 ID（如果有），否则新生成
	taskID := ""
	if relayInfo.TaskRelayInfo != nil && relayInfo.TaskRelayInfo.PublicTaskID != "" {
		taskID = relayInfo.TaskRelayInfo.PublicTaskID
	} else {
		taskID = GenerateTaskID()
	}

	t := &Task{
		TaskID:      taskID,
		UserId:      relayInfo.UserId,
		Group:       relayInfo.UsingGroup,
		SubmitTime:  time.Now().Unix(),
		Status:      TaskStatusNotStart,
		Progress:    "0%",
		ChannelId:   relayInfo.ChannelId,
		Platform:    platform,
		Properties:  properties,
		PrivateData: privateData,
	}
	return t
}

func TaskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Omit("channel_id").Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func TaskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetTimedOutUnfinishedTasks(cutoffUnix int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status NOT IN ?", []string{TaskStatusFailure, TaskStatusSuccess}).
		Where("submit_time < ?", cutoffUnix).
		Order("submit_time").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetAllUnFinishSyncTasks(limit int) []*Task {
	var tasks []*Task
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").Where("status != ?", TaskStatusFailure).Where("status != ?", TaskStatusSuccess).Limit(limit).Order("id").Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedSyncTasks reports whether at least one async (Suno/video) task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the async_task_poll system task needs to run; when no task is pending
// the scheduler skips creating a row entirely.
func HasUnfinishedSyncTasks() bool {
	var id int64
	err := DB.Model(&Task{}).
		Where("progress != ?", "100%").
		Where("status != ?", TaskStatusFailure).
		Where("status != ?", TaskStatusSuccess).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	var err error
	err = DB.Where("user_id = ? and task_id = ?", userId, taskId).
		First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var task []*Task
	var err error
	err = DB.Where("user_id = ? and task_id in (?)", userId, taskIds).
		Find(&task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (Task *Task) Insert() error {
	var err error
	err = DB.Create(Task).Error
	return err
}

type taskSnapshot struct {
	Status     TaskStatus
	Progress   string
	StartTime  int64
	FinishTime int64
	FailReason string
	ResultURL  string
	Data       json.RawMessage
}

func (s taskSnapshot) Equal(other taskSnapshot) bool {
	return s.Status == other.Status &&
		s.Progress == other.Progress &&
		s.StartTime == other.StartTime &&
		s.FinishTime == other.FinishTime &&
		s.FailReason == other.FailReason &&
		s.ResultURL == other.ResultURL &&
		bytes.Equal(s.Data, other.Data)
}

func (t *Task) Snapshot() taskSnapshot {
	return taskSnapshot{
		Status:     t.Status,
		Progress:   t.Progress,
		StartTime:  t.StartTime,
		FinishTime: t.FinishTime,
		FailReason: t.FailReason,
		ResultURL:  t.PrivateData.ResultURL,
		Data:       t.Data,
	}
}

func (Task *Task) Update() error {
	var err error
	err = DB.Save(Task).Error
	return err
}

func (t *Task) UpdateQuota() error {
	return DB.Model(t).Update("quota", t.Quota).Error
}

// AdvanceBillingQuota persists one organization billing transition. The
// revision makes a repeated quota cycle a distinct operation while the CAS
// ensures concurrent pollers cannot both apply token and usage side effects.
func (t *Task) AdvanceBillingQuota(expectedQuota int, expectedRevision int64, quota int) (bool, error) {
	return t.advanceBillingQuotaTx(DB, expectedQuota, expectedRevision, quota, false)
}

type OrganizationTaskBillingMutationParams struct {
	TaskId                    int64
	UserId                    int
	OrganizationId            int
	OrganizationReservationId int64
	SubscriptionId            int
	LegacyOrganizationWallet  bool
	TokenId                   int
	ChannelId                 int
	ExpectedQuota             int
	ExpectedRevision          int64
	ActualQuota               int
	OperationId               string
}

type OrganizationTaskBillingMutationResult struct {
	Applied    bool
	QuotaDelta int
	QuotaClamp *common.QuotaClamp
}

var errOrganizationTaskBillingStale = errors.New("organization task billing state changed")

// ApplyOrganizationTaskBillingMutation commits the authoritative task billing
// state in one main-database transaction. Redis and logs are projections and
// are deliberately updated only after this function commits.
func ApplyOrganizationTaskBillingMutation(params OrganizationTaskBillingMutationParams) (OrganizationTaskBillingMutationResult, error) {
	hasReservation := params.OrganizationReservationId > 0
	hasSubscription := params.SubscriptionId > 0
	fundingModes := 0
	if hasReservation {
		fundingModes++
	}
	if hasSubscription {
		fundingModes++
	}
	if params.LegacyOrganizationWallet {
		fundingModes++
	}
	if DB == nil || params.TaskId <= 0 || params.UserId <= 0 || params.OrganizationId <= 0 ||
		params.OrganizationReservationId < 0 || params.SubscriptionId < 0 || params.TokenId < 0 || params.ChannelId < 0 ||
		params.ExpectedQuota <= 0 || params.ExpectedQuota >= common.MaxQuota || params.ExpectedRevision < 0 ||
		params.ActualQuota < 0 || params.ActualQuota >= common.MaxQuota || params.ActualQuota == params.ExpectedQuota ||
		strings.TrimSpace(params.OperationId) == "" || len(params.OperationId) > 64 ||
		fundingModes != 1 {
		return OrganizationTaskBillingMutationResult{}, ErrOrganizationAccountingInvalid
	}

	quotaDelta := params.ActualQuota - params.ExpectedQuota
	result := OrganizationTaskBillingMutationResult{QuotaDelta: quotaDelta}
	walletDelta := int64(0)
	walletProjectionNeeded := false
	tokenKey := ""
	err := DB.Transaction(func(tx *gorm.DB) error {
		fundingAlreadyApplied := false
		actor := OrganizationAccountingActor{
			Kind:   OrganizationAccountingActorSystem,
			Policy: "async_task_billing",
		}
		if hasSubscription {
			if err := adjustOrganizationMemberConsumptionTx(tx, params.OrganizationId, params.UserId, int64(quotaDelta)); err != nil {
				return err
			}
			if err := postConsumeUserSubscriptionDeltaTx(tx, params.SubscriptionId, params.UserId, int64(quotaDelta), true); err != nil {
				return err
			}
		} else if hasReservation {
			if params.ActualQuota == 0 {
				refunded, delta, err := refundOrganizationWalletQuotaTx(tx, OrganizationWalletRefundParams{
					ReservationId:  params.OrganizationReservationId,
					IdempotencyKey: params.OperationId,
					RequestId:      params.OperationId,
					Actor:          actor,
				})
				if err != nil {
					return err
				}
				if refunded.Reservation.OrganizationId != params.OrganizationId || refunded.Reservation.UserId != params.UserId {
					return ErrOrganizationIdentityInvalid
				}
				if refunded.Reservation.ReservedQuota != int64(params.ExpectedQuota) {
					return ErrOrganizationReservationState
				}
				walletDelta = delta
				fundingAlreadyApplied = refunded.Accounting.AlreadyApplied
			} else {
				expectedQuota := int64(params.ExpectedQuota)
				adjusted, delta, err := applyOrganizationWalletQuotaTargetTx(
					tx,
					OrganizationWalletSettleParams{
						ReservationId:  params.OrganizationReservationId,
						ActualQuota:    int64(params.ActualQuota),
						IdempotencyKey: params.OperationId,
						RequestId:      params.OperationId,
						Actor:          actor,
					},
					&expectedQuota,
					OrganizationWalletReservationSettled,
					OrganizationLedgerAdjust,
					"organization.wallet.adjust",
				)
				if err != nil {
					return err
				}
				if adjusted.Reservation.OrganizationId != params.OrganizationId || adjusted.Reservation.UserId != params.UserId {
					return ErrOrganizationIdentityInvalid
				}
				walletDelta = delta
				fundingAlreadyApplied = adjusted.Accounting.AlreadyApplied
			}
		} else {
			funding, err := adjustLegacyOrganizationTaskWalletFundingTx(tx, LegacyOrganizationTaskWalletParams{
				TaskId: params.TaskId, UserId: params.UserId, OrganizationId: params.OrganizationId,
				ExpectedQuota: params.ExpectedQuota, ExpectedRevision: params.ExpectedRevision,
				ActualQuota: params.ActualQuota, OperationId: params.OperationId,
			})
			if err != nil {
				return err
			}
			walletDelta = funding.WalletDelta
			fundingAlreadyApplied = funding.AlreadyApplied
		}

		// The funding state machine normally holds this row already. Re-lock it
		// on an idempotent replay so every path keeps user -> task -> token ->
		// channel ordering and cannot invert a fresh accounting mutation.
		var user User
		if err := lockForUpdate(tx).
			Select("id", "organization_id", "used_quota").
			Where("id = ?", params.UserId).
			First(&user).Error; err != nil {
			return err
		}
		if user.OrganizationId != params.OrganizationId {
			return ErrOrganizationIdentityInvalid
		}

		var task Task
		if err := lockForUpdate(tx).Where("id = ?", params.TaskId).First(&task).Error; err != nil {
			return err
		}
		if task.UserId != params.UserId || task.OrganizationId != params.OrganizationId ||
			task.PrivateData.TokenId != params.TokenId || task.ChannelId != params.ChannelId ||
			task.LegacyOrganizationWallet != params.LegacyOrganizationWallet {
			return ErrOrganizationIdentityInvalid
		}
		if hasSubscription {
			if task.PrivateData.BillingSource != taskBillingSourceSubscription ||
				task.PrivateData.SubscriptionId != params.SubscriptionId ||
				!task.PrivateData.OrganizationSubscriptionMetered ||
				task.PrivateData.OrganizationReservationId != 0 || task.LegacyOrganizationWallet {
				return ErrOrganizationIdentityInvalid
			}
		} else if task.PrivateData.BillingSource == taskBillingSourceSubscription {
			return ErrOrganizationIdentityInvalid
		} else if hasReservation {
			if task.PrivateData.OrganizationReservationId != params.OrganizationReservationId {
				return ErrOrganizationIdentityInvalid
			}
		} else if task.PrivateData.OrganizationReservationId != 0 || !task.LegacyOrganizationWallet {
			return ErrOrganizationIdentityInvalid
		}

		advanced, err := task.advanceBillingQuotaTx(tx, params.ExpectedQuota, params.ExpectedRevision, params.ActualQuota, params.LegacyOrganizationWallet)
		if err != nil {
			return err
		}
		if !advanced {
			if fundingAlreadyApplied {
				return nil
			}
			return errOrganizationTaskBillingStale
		}
		userUsedQuota, userQuotaClamp := common.SaturatingInt32CounterAddChecked(user.UsedQuota, quotaDelta)
		result.QuotaClamp = userQuotaClamp

		if params.TokenId > 0 {
			var token Token
			err := lockForUpdate(tx.Unscoped()).
				Select("id", "key", "used_quota").
				Where("id = ?", params.TokenId).
				First(&token).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				tokenUsedQuota, tokenQuotaClamp := common.SaturatingInt32CounterAddChecked(token.UsedQuota, quotaDelta)
				if result.QuotaClamp == nil {
					result.QuotaClamp = tokenQuotaClamp
				}
				tokenUpdate := tx.Unscoped().Model(&Token{}).
					Where("id = ?", token.Id).
					Updates(map[string]interface{}{
						"remain_quota":  gorm.Expr("remain_quota - ?", quotaDelta),
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
			}
		}

		userUsage := tx.Model(&User{}).
			Where("id = ? AND organization_id = ?", params.UserId, params.OrganizationId).
			Update("used_quota", userUsedQuota)
		if userUsage.Error != nil {
			return userUsage.Error
		}
		if params.ChannelId > 0 {
			channelUsage := tx.Model(&Channel{}).
				Where("id = ?", params.ChannelId).
				Update("used_quota", gorm.Expr("used_quota + ?", quotaDelta))
			if channelUsage.Error != nil {
				return channelUsage.Error
			}
		}

		walletProjectionNeeded = !fundingAlreadyApplied
		result.Applied = true
		return nil
	})
	if errors.Is(err, errOrganizationTaskBillingStale) {
		return OrganizationTaskBillingMutationResult{QuotaDelta: quotaDelta}, nil
	}
	if err != nil && hasSubscription && quotaDelta < 0 {
		var persisted Task
		lookupErr := DB.First(&persisted, params.TaskId).Error
		if lookupErr == nil && persisted.UserId == params.UserId && persisted.OrganizationId == params.OrganizationId &&
			persisted.ChannelId == params.ChannelId && persisted.PrivateData.TokenId == params.TokenId &&
			persisted.PrivateData.BillingSource == taskBillingSourceSubscription &&
			persisted.PrivateData.SubscriptionId == params.SubscriptionId &&
			persisted.PrivateData.OrganizationSubscriptionMetered &&
			persisted.BillingRevision > params.ExpectedRevision {
			return OrganizationTaskBillingMutationResult{QuotaDelta: quotaDelta}, nil
		}
	}
	if err != nil {
		return OrganizationTaskBillingMutationResult{}, err
	}
	if !result.Applied {
		return result, nil
	}
	if walletProjectionNeeded {
		syncOrganizationAccountingQuotaCache(params.UserId, walletDelta, OrganizationLedgerAdjust)
	}
	if tokenKey != "" && common.RedisEnabled {
		_, cacheErr := cacheApplyTokenQuotaDelta(params.TokenId, tokenKey, int64(-quotaDelta))
		if cacheErr != nil {
			common.SysLog("failed to sync organization task token quota cache: " + cacheErr.Error())
		}
	}
	return result, nil
}

func (t *Task) advanceBillingQuotaTx(tx *gorm.DB, expectedQuota int, expectedRevision int64, quota int, legacyOrganizationWalletOnly bool) (bool, error) {
	if tx == nil || t == nil || t.ID <= 0 || expectedQuota < 0 || expectedRevision < 0 || quota < 0 || quota >= common.MaxQuota {
		return false, ErrOrganizationAccountingInvalid
	}
	query := tx.Model(&Task{}).
		Where("id = ? AND quota = ?", t.ID, expectedQuota).
		Where("billing_revision = ? OR (billing_revision IS NULL AND ? = 0)", expectedRevision, expectedRevision)
	if legacyOrganizationWalletOnly {
		if t.UserId <= 0 || t.OrganizationId <= 0 || !t.LegacyOrganizationWallet {
			return false, ErrOrganizationAccountingInvalid
		}
		query = query.Where("user_id = ? AND organization_id = ? AND legacy_organization_wallet = ?", t.UserId, t.OrganizationId, true)
	}
	result := query.
		Updates(map[string]interface{}{
			"quota":            quota,
			"billing_revision": expectedRevision + 1,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	t.Quota = quota
	t.BillingRevision = expectedRevision + 1
	return true, nil
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus. MySQL commonly
// reports changed rows rather than matched rows, so a same-value no-op update
// can also return false even when the status predicate still matched.
//
// Uses Model().Select("*").Updates() instead of Save() because GORM's Save
// falls back to INSERT ON CONFLICT when the WHERE-guarded UPDATE matches
// zero rows, which silently bypasses the CAS guard.
func (t *Task) UpdateWithStatus(fromStatus TaskStatus) (bool, error) {
	result := DB.Model(t).Where("status = ?", fromStatus).Select("*").Updates(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// TaskBulkUpdateByID performs an unconditional bulk UPDATE by primary key IDs.
// WARNING: This function has NO CAS (Compare-And-Swap) guard — it will overwrite
// any concurrent status changes. DO NOT use in billing/quota lifecycle flows
// (e.g., timeout, success, failure transitions that trigger refunds or settlements).
// For status transitions that involve billing, use Task.UpdateWithStatus() instead.
func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("id in (?)", ids).
		Updates(params).Error
}

type TaskQuotaUsage struct {
	Mode  string  `json:"mode"`
	Count float64 `json:"count"`
}

// TaskCountAllTasks returns total tasks that match the given query params (admin usage)
func TaskCountAllTasks(queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// TaskCountAllUserTask returns total tasks for given user
func TaskCountAllUserTask(userId int, queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{}).Where("user_id = ?", userId)
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
func (t *Task) ToOpenAIVideo() *dto.OpenAIVideo {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = t.TaskID
	openAIVideo.Status = t.Status.ToVideoStatus()
	openAIVideo.Model = t.Properties.OriginModelName
	openAIVideo.SetProgressStr(t.Progress)
	openAIVideo.CreatedAt = t.CreatedAt
	openAIVideo.CompletedAt = t.UpdatedAt
	openAIVideo.SetMetadata("url", t.GetResultURL())
	return openAIVideo
}
