package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type OrganizationAuthorizationChange struct {
	UserId      int
	AuthVersion int64
}

// AdvanceOrganizationUserAuthorizationWithTx is the shared auth-version hook
// for organization role, membership status and ownership changes made inside
// a caller-owned transaction. Call FinalizeOrganizationAuthorizationChanges
// only after that transaction commits.
func AdvanceOrganizationUserAuthorizationWithTx(tx *gorm.DB, userId int) (OrganizationAuthorizationChange, error) {
	next, err := IncrementUserAuthVersionWithTx(tx, userId)
	if err != nil {
		return OrganizationAuthorizationChange{}, err
	}
	return OrganizationAuthorizationChange{UserId: userId, AuthVersion: next}, nil
}

// FinalizeOrganizationAuthorizationChanges publishes committed cache floors,
// revokes browser sessions and invalidates every API-key cache snapshot.
func FinalizeOrganizationAuthorizationChanges(changes []OrganizationAuthorizationChange, reason string) error {
	if reason == "" {
		reason = "organization_authorization_changed"
	}
	if len(reason) > 64 {
		reason = reason[:64]
	}
	var errs []error
	seen := make(map[int]struct{}, len(changes))
	for _, change := range changes {
		if change.UserId <= 0 || change.AuthVersion <= 0 {
			errs = append(errs, fmt.Errorf("invalid organization authorization change"))
			continue
		}
		if _, ok := seen[change.UserId]; ok {
			continue
		}
		seen[change.UserId] = struct{}{}
		if err := PublishUserAuthCache(change.UserId); err != nil {
			errs = append(errs, fmt.Errorf("publish user %d organization authorization: %w", change.UserId, err))
		}
		if _, err := RevokeAllUserSessions(change.UserId, reason); err != nil {
			errs = append(errs, fmt.Errorf("revoke user %d sessions: %w", change.UserId, err))
		}
		if err := InvalidateUserTokensCache(change.UserId); err != nil {
			errs = append(errs, fmt.Errorf("invalidate user %d token cache: %w", change.UserId, err))
		}
	}
	return errors.Join(errs...)
}

func isOrganizationRole(role OrganizationRole) bool {
	switch role {
	case OrganizationRoleOwner, OrganizationRoleAdmin, OrganizationRoleMember:
		return true
	default:
		return false
	}
}
