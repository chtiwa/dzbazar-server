package services

import (
	"errors"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrInvalidAction is returned when the action isn't a known permission_actions row.
var ErrInvalidAction = errors.New("invalid action")

// roleDefault is the shop-role → action default, backed by role_action_defaults.
// No row means deny — every gated action must have a seeded row per role.
// ponytail: one small indexed lookup per gated request, same cost as the
// override read below; cache if this ever shows up hot in profiling.
func roleDefault(role, action string) bool {
	var d models.RoleActionDefault
	if err := initializers.DB.Where("role = ? AND action = ?", role, action).First(&d).Error; err != nil {
		return false
	}
	return d.Allow
}

// MemberCan resolves the role default then applies a per-member override if
// one exists. memberID is nil on the impersonation path (no membership row),
// where role is always "owner" and no override can apply.
func MemberCan(memberID *uuid.UUID, role, action string) bool {
	def := roleDefault(role, action)
	if memberID == nil {
		return def
	}
	var ov models.ShopMemberPermission
	err := initializers.DB.
		Where("shop_member_id = ? AND action = ?", *memberID, action).
		First(&ov).Error
	if err != nil {
		return def
	}
	return ov.Allow
}

// ResolvedPermissions returns every action → allowed for a member, in two
// queries (role defaults + overrides) instead of one MemberCan per action.
func ResolvedPermissions(memberID *uuid.UUID, role string) (map[string]bool, error) {
	var actions []models.PermissionAction
	if err := initializers.DB.Find(&actions).Error; err != nil {
		return nil, err
	}
	var defaults []models.RoleActionDefault
	if err := initializers.DB.Where("role = ?", role).Find(&defaults).Error; err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(actions))
	for _, a := range actions {
		out[a.Name] = false
	}
	for _, d := range defaults {
		if _, ok := out[d.Action]; ok {
			out[d.Action] = d.Allow
		}
	}
	if memberID != nil {
		overrides, err := ListOverrides(*memberID)
		if err != nil {
			return nil, err
		}
		for _, o := range overrides {
			if _, ok := out[o.Action]; ok {
				out[o.Action] = o.Allow
			}
		}
	}
	return out, nil
}

func validAction(action string) bool {
	var n int64
	initializers.DB.Model(&models.PermissionAction{}).Where("name = ?", action).Count(&n)
	return n > 0
}

// SetOverride upserts a per-member allow/deny grant for action.
func SetOverride(memberID uuid.UUID, action string, allow bool) error {
	if !validAction(action) {
		return ErrInvalidAction
	}
	return initializers.DB.
		Where("shop_member_id = ? AND action = ?", memberID, action).
		Assign(models.ShopMemberPermission{Allow: allow}).
		FirstOrCreate(&models.ShopMemberPermission{ShopMemberID: memberID, Action: action, Allow: allow}).Error
}

// ClearOverride removes a member's override for action, reverting to role default.
func ClearOverride(memberID uuid.UUID, action string) error {
	return initializers.DB.
		Where("shop_member_id = ? AND action = ?", memberID, action).
		Delete(&models.ShopMemberPermission{}).Error
}

// ListOverrides returns every override row for a member.
func ListOverrides(memberID uuid.UUID) ([]models.ShopMemberPermission, error) {
	var overrides []models.ShopMemberPermission
	err := initializers.DB.Where("shop_member_id = ?", memberID).Find(&overrides).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return overrides, nil
	}
	return overrides, err
}

// ActionForRole is a grantable action annotated with this role's default,
// so the admin UI can pre-check a box without a second round trip.
type ActionForRole struct {
	Name         string `json:"name"`
	Resource     string `json:"resource"`
	Label        string `json:"label"`
	DefaultAllow bool   `json:"defaultAllow"`
}

// ListActionsForRole returns every grantable action plus role's default,
// ordered by resource so the UI can render stable section groups.
func ListActionsForRole(role string) ([]ActionForRole, error) {
	var out []ActionForRole
	err := initializers.DB.
		Table("permission_actions AS pa").
		Select("pa.name, pa.resource, pa.label, COALESCE(rad.allow, false) AS default_allow").
		Joins("LEFT JOIN role_action_defaults rad ON rad.action = pa.name AND rad.role = ?", role).
		Order("pa.resource, pa.name").
		Scan(&out).Error
	return out, err
}
