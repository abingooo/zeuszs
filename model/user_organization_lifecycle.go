package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// These errors are deliberately defined in model so every user deletion and
// platform-status mutation path (controllers, jobs, and internal services)
// shares the same fail-closed organization invariants.
var (
	ErrOrganizationOwnerDeletionForbidden = errors.New("organization owner must be transferred before deletion")
	ErrOrganizationOwnerDisableForbidden  = errors.New("organization owner must be transferred before disabling")
	ErrOrganizationMemberFundsOutstanding = errors.New("organization member has outstanding organization-funded quota")
)

// validateUserOrganizationStatusChangeTx protects the organization owner from
// being disabled. Ownership is intentionally resolved from locked database
// rows, never from a request payload or a cached user snapshot.
func validateUserOrganizationStatusChangeTx(tx *gorm.DB, current *User, nextStatus int) error {
	if tx == nil || current == nil || nextStatus != common.UserStatusDisabled || current.Status == nextStatus {
		return nil
	}
	owner, err := organizationOwnerForUserTx(tx, current)
	if err != nil {
		return err
	}
	if owner {
		return ErrOrganizationOwnerDisableForbidden
	}
	return nil
}

// validateUserOrganizationDeletionTx verifies that deleting a user cannot
// orphan organization ownership or lose recoverable organization funds. The
// caller's transaction must remain open through the actual delete; accounting
// mutations lock the same user/member-fund rows, so a concurrent allocation
// cannot race past this check.
func validateUserOrganizationDeletionTx(tx *gorm.DB, userID int) error {
	if tx == nil || userID <= 0 {
		return errors.New("invalid user deletion")
	}
	var current User
	if err := lockForUpdate(tx.Unscoped()).Where("id = ?", userID).First(&current).Error; err != nil {
		return err
	}

	owner, err := organizationOwnerForUserTx(tx, &current)
	if err != nil {
		return err
	}
	if owner {
		return ErrOrganizationOwnerDeletionForbidden
	}
	if current.OrganizationId <= 0 {
		return nil
	}

	// A reservation holds a split of the user's wallet and must be settled or
	// refunded before the identity can disappear. Locking the user above
	// serializes this check with the reservation state machine.
	if tx.Migrator().HasTable(&OrganizationWalletReservation{}) {
		var reservations []OrganizationWalletReservation
		// The user row above is the serialization fence for every reservation
		// mutation. Do not lock the reservation after locking the user: settlement
		// and refund use reservation -> user order, and taking the inverse order
		// here can deadlock on MySQL/PostgreSQL.
		if err := tx.Unscoped().
			Where("user_id = ? AND status = ?", userID, OrganizationWalletReservationReserved).
			Limit(1).Find(&reservations).Error; err != nil {
			return err
		}
		if len(reservations) > 0 {
			return ErrOrganizationMemberFundsOutstanding
		}
	}

	// A settled asynchronous task can still become refundable until the upstream
	// work reaches a terminal state. Keep the user row as the serialization fence
	// so a later failure can restore the wallet and usage counters against the
	// same tenant identity. Reserved Midjourney submissions are included even if
	// an upstream response already marked them failed but recovery has not run.
	if tx.Migrator().HasTable(&Midjourney{}) {
		var pending []Midjourney
		if err := tx.Unscoped().
			Select("id").
			Where("user_id = ? AND quota > 0", userID).
			Where("billing_status = ? OR status IS NULL OR status <> ?", MidjourneyBillingStatusReserved, "SUCCESS").
			Limit(1).
			Find(&pending).Error; err != nil {
			return err
		}
		if len(pending) > 0 {
			return ErrOrganizationMemberFundsOutstanding
		}
	}
	if tx.Migrator().HasTable(&Task{}) {
		var pending []Task
		if err := tx.Unscoped().
			Select("id").
			Where("user_id = ? AND organization_id = ? AND quota > 0", userID, current.OrganizationId).
			Where("status IS NULL OR status <> ?", TaskStatusSuccess).
			Limit(1).
			Find(&pending).Error; err != nil {
			return err
		}
		if len(pending) > 0 {
			return ErrOrganizationMemberFundsOutstanding
		}
	}

	// RecoverableQuota is the portion of the single wallet that still belongs
	// to the organization. It must be explicitly recovered by an authorized
	// organization administrator before deleting the member. A missing legacy
	// row is tolerated for pre-organization databases, but an existing row with
	// an invalid/positive balance fails closed.
	if tx.Migrator().HasTable(&OrganizationMemberFund{}) {
		var fund OrganizationMemberFund
		err := lockForUpdate(tx.Unscoped()).Where("organization_id = ? AND user_id = ?", current.OrganizationId, userID).First(&fund).Error
		if err == nil {
			if fund.RecoverableQuota != 0 {
				return ErrOrganizationMemberFundsOutstanding
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return nil
}

// organizationOwnerForUserTx resolves ownership from both sides of the
// relationship. Treat either an owner membership role or an organization
// owner_user_id pointer as ownership so corrupt/partially migrated rows fail
// closed instead of allowing the last owner to disappear.
func organizationOwnerForUserTx(tx *gorm.DB, current *User) (bool, error) {
	if tx == nil || current == nil {
		return false, errors.New("invalid organization owner lookup")
	}
	if current.OrganizationRole == OrganizationRoleOwner {
		return true, nil
	}
	if current.OrganizationId <= 0 || !tx.Migrator().HasTable(&Organization{}) {
		return false, nil
	}
	var organization Organization
	// Do not lock the organization after the caller has locked the user: all
	// organization management services lock in organization -> user order. A
	// transfer to this user still has to acquire the locked user row and will
	// fail after a concurrent deletion commits.
	if err := tx.Unscoped().
		Select("id", "owner_user_id").
		Where("id = ?", current.OrganizationId).
		First(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Once a user carries a tenant id, a missing tenant is an invalid
			// identity. Deletion/status changes must not hide that corruption.
			return false, ErrOrganizationIdentityInvalid
		}
		return false, err
	}
	return organization.OwnerUserId == current.Id, nil
}
