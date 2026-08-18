package services

import (
	"errors"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

// ErrInvalidPlatformRole is returned when newRole isn't one of the values
// documented on models.User.PlatformRole ("" | "support" | "super_admin").
var ErrInvalidPlatformRole = errors.New("invalid platform role")

// ErrSelfDemotion is returned when a super_admin tries to change their own
// PlatformRole away from super_admin — allowing it would lock them out of
// the entire super-admin panel (RequirePlatformRole gates every route on
// this field) with nobody left in the UI able to undo it.
var ErrSelfDemotion = errors.New("cannot change your own super_admin role")

// validPlatformRoles mirrors the values documented on models.User.PlatformRole.
var validPlatformRoles = map[string]bool{
	"":            true, // regular user, no platform access
	"support":     true,
	"super_admin": true,
}

// SetPlatformRole changes a user's platform-wide role. actingUser is who's
// making the change, used only to block a super_admin from demoting/removing
// their own role. Returns the updated user plus the role it had before the
// change, for audit logging.
func SetPlatformRole(actingUser models.User, targetUserID uuid.UUID, newRole string) (models.User, string, error) {
	if !validPlatformRoles[newRole] {
		return models.User{}, "", ErrInvalidPlatformRole
	}

	var target models.User
	if err := initializers.DB.First(&target, "id = ?", targetUserID).Error; err != nil {
		return target, "", err
	}

	previousRole := target.PlatformRole

	if target.ID == actingUser.ID && previousRole == "super_admin" && newRole != "super_admin" {
		return target, previousRole, ErrSelfDemotion
	}

	if err := initializers.DB.Model(&target).Update("platform_role", newRole).Error; err != nil {
		return target, previousRole, err
	}
	target.PlatformRole = newRole
	return target, previousRole, nil
}
