package model

import "errors"

// ErrOrganizationIdentityInvalid is returned when a resource cannot be
// assigned to a valid server-derived tenant. It lives in model so persistence
// hooks can fail closed without introducing a model-to-service dependency.
var ErrOrganizationIdentityInvalid = errors.New("organization identity is invalid")

const (
	DefaultOrganizationSystemKey = "default"
	DefaultOrganizationName      = "Default Organization"
)

type OrganizationStatus string

const (
	OrganizationStatusActive     OrganizationStatus = "active"
	OrganizationStatusDisabled   OrganizationStatus = "disabled"
	OrganizationStatusDissolving OrganizationStatus = "dissolving"
	OrganizationStatusDissolved  OrganizationStatus = "dissolved"
)

type OrganizationRole string

const (
	OrganizationRoleOwner  OrganizationRole = "owner"
	OrganizationRoleAdmin  OrganizationRole = "admin"
	OrganizationRoleMember OrganizationRole = "member"
)

type OrganizationMemberStatus string

const (
	OrganizationMemberStatusActive   OrganizationMemberStatus = "active"
	OrganizationMemberStatusDisabled OrganizationMemberStatus = "disabled"
)

type OrganizationInviteStatus string

const (
	OrganizationInviteStatusActive   OrganizationInviteStatus = "active"
	OrganizationInviteStatusDisabled OrganizationInviteStatus = "disabled"
)

const (
	OrganizationLedgerStatusCommitted = "committed"
)

// Organization is the tenant boundary. SystemKey is nullable so only the
// platform-owned default organization occupies the unique "default" value.
type Organization struct {
	Id                   int                `json:"id"`
	Name                 string             `json:"name" gorm:"type:varchar(128);not null;index"`
	SystemKey            *string            `json:"system_key,omitempty" gorm:"type:varchar(32);uniqueIndex"`
	Status               OrganizationStatus `json:"status" gorm:"type:varchar(16);not null;index"`
	OwnerUserId          int                `json:"owner_user_id" gorm:"not null;uniqueIndex"`
	AllowMemberTopup     bool               `json:"allow_member_topup" gorm:"not null"`
	PolicyVersion        int64              `json:"policy_version" gorm:"type:bigint;not null"`
	LegacyMainBackfillAt int64              `json:"-" gorm:"type:bigint;not null"`
	LegacyLogBackfillAt  int64              `json:"-" gorm:"type:bigint;not null"`
	CreatedAt            int64              `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64              `json:"updated_at" gorm:"autoUpdateTime"`
}

type OrganizationInvite struct {
	Id             int                      `json:"id"`
	OrganizationId int                      `json:"organization_id" gorm:"not null;index"`
	CodeHash       string                   `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	CreationToken  string                   `json:"-" gorm:"type:varchar(64)"`
	CodePrefix     string                   `json:"code_prefix" gorm:"type:varchar(16);not null"`
	Status         OrganizationInviteStatus `json:"status" gorm:"type:varchar(16);not null;index"`
	MaxUses        int                      `json:"max_uses" gorm:"not null"`
	UsedCount      int                      `json:"used_count" gorm:"not null"`
	ExpiresAt      int64                    `json:"expires_at" gorm:"type:bigint;not null;index"`
	DefaultRole    OrganizationRole         `json:"default_role" gorm:"type:varchar(16);not null"`
	CreatedBy      int                      `json:"created_by" gorm:"not null;index"`
	CreatedAt      int64                    `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64                    `json:"updated_at" gorm:"autoUpdateTime"`
}

type OrganizationInviteUse struct {
	Id                   int    `json:"id"`
	OrganizationInviteId int    `json:"organization_invite_id" gorm:"not null;uniqueIndex:idx_organization_invite_user"`
	OrganizationId       int    `json:"organization_id" gorm:"not null;index"`
	UserId               int    `json:"user_id" gorm:"not null;uniqueIndex;uniqueIndex:idx_organization_invite_user"`
	RequestId            string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	UsedAt               int64  `json:"used_at" gorm:"autoCreateTime"`
}

// OrganizationFundAccount is an organization budget pool, not a second user
// wallet. Quota is int64 so aggregation cannot overflow the int32 user wallet.
type OrganizationFundAccount struct {
	Id             int64 `json:"id"`
	OrganizationId int   `json:"organization_id" gorm:"not null;uniqueIndex"`
	Quota          int64 `json:"quota" gorm:"type:bigint;not null"`
	CreatedAt      int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// OrganizationMemberFund tracks the organization-funded, still-recoverable
// subset of a member's single wallet plus the member's consumption limit.
type OrganizationMemberFund struct {
	Id               int64  `json:"id"`
	OrganizationId   int    `json:"organization_id" gorm:"not null;uniqueIndex:idx_organization_member_fund"`
	UserId           int    `json:"user_id" gorm:"not null;uniqueIndex:idx_organization_member_fund"`
	RecoverableQuota int64  `json:"recoverable_quota" gorm:"type:bigint;not null"`
	ConsumptionLimit *int64 `json:"consumption_limit,omitempty" gorm:"type:bigint"`
	ConsumedQuota    int64  `json:"consumed_quota" gorm:"type:bigint;not null"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// OrganizationQuotaLedger is an append-only record committed in the same
// transaction as the balances it describes.
type OrganizationQuotaLedger struct {
	Id                    int64  `json:"id"`
	OrganizationId        int    `json:"organization_id" gorm:"not null;index"`
	UserId                int    `json:"user_id" gorm:"not null;index"`
	ProjectId             *int   `json:"project_id,omitempty" gorm:"index"`
	Operation             string `json:"operation" gorm:"type:varchar(32);not null;index"`
	SourceType            string `json:"source_type" gorm:"type:varchar(32);not null;index"`
	SourceId              string `json:"source_id" gorm:"type:varchar(128);not null;index"`
	ActorUserId           int    `json:"actor_user_id" gorm:"not null;index"`
	IdempotencyKey        string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	Fingerprint           string `json:"-" gorm:"type:char(64);not null"`
	RequestId             string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	UserQuotaDelta        int64  `json:"user_quota_delta" gorm:"type:bigint;not null"`
	PoolQuotaDelta        int64  `json:"pool_quota_delta" gorm:"type:bigint;not null"`
	RecoverableQuotaDelta int64  `json:"recoverable_quota_delta" gorm:"type:bigint;not null"`
	UserQuotaAfter        int64  `json:"user_quota_after" gorm:"type:bigint;not null"`
	PoolQuotaAfter        int64  `json:"pool_quota_after" gorm:"type:bigint;not null"`
	RecoverableQuotaAfter int64  `json:"recoverable_quota_after" gorm:"type:bigint;not null"`
	RelatedLedgerId       *int64 `json:"related_ledger_id,omitempty" gorm:"index"`
	Status                string `json:"status" gorm:"type:varchar(16);not null;index"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type OrganizationAuditEvent struct {
	Id             int64  `json:"id"`
	OrganizationId int    `json:"organization_id" gorm:"not null;index"`
	ActorUserId    int    `json:"actor_user_id" gorm:"not null;index"`
	Action         string `json:"action" gorm:"type:varchar(64);not null;index"`
	TargetType     string `json:"target_type" gorm:"type:varchar(32);not null"`
	TargetId       string `json:"target_id" gorm:"type:varchar(128);not null"`
	RequestId      string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	Metadata       string `json:"metadata" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}
