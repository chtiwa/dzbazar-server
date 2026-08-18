package services

import (
	"errors"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

// ErrInvalidRole is returned when role isn't a known shop_roles row.
var ErrInvalidRole = errors.New("invalid role")

// PermissionCatalog is the full RBAC catalog: every gate-able action plus
// every role's default for it. Global, non-shop-scoped data — safe to expose
// read-only to support, not just super_admin.
type PermissionCatalog struct {
	Actions  []models.PermissionAction  `json:"actions"`
	Defaults []models.RoleActionDefault `json:"defaults"`
}

// GetPermissionCatalog returns the full action catalog and every
// role_action_defaults row, so the UI can build the
// owner/moderator/confirmation x action matrix itself.
func GetPermissionCatalog() (PermissionCatalog, error) {
	var out PermissionCatalog
	if err := initializers.DB.Order("resource, name").Find(&out.Actions).Error; err != nil {
		return out, err
	}
	if err := initializers.DB.Order("role, action").Find(&out.Defaults).Error; err != nil {
		return out, err
	}
	return out, nil
}

// RoleDeviation is one shop_member_permissions row whose Allow disagrees with
// the member's current role default — an override that's actually doing
// something right now, as opposed to a stale no-op left over from a
// since-changed default.
type RoleDeviation struct {
	ShopMemberID uuid.UUID `json:"shopMemberId"`
	ShopID       uuid.UUID `json:"shopId"`
	ShopName     string    `json:"shopName"`
	UserID       uuid.UUID `json:"userId"`
	UserEmail    string    `json:"userEmail"`
	UserName     string    `json:"userName"`
	Role         string    `json:"role"`
	Action       string    `json:"action"`
	Resource     string    `json:"resource"`
	Label        string    `json:"label"`
	Override     bool      `json:"override"`
	DefaultAllow bool      `json:"defaultAllow"`
}

// ListRoleDeviations returns every member-level override that actually
// disagrees with that member's current role default — the support/ops
// diagnostic for "why does this merchant employee have unusual access".
// Overrides that now happen to match the (possibly since-changed) default
// aren't a deviation and are omitted.
func ListRoleDeviations() ([]RoleDeviation, error) {
	var out []RoleDeviation
	err := initializers.DB.
		Table("shop_member_permissions AS smp").
		Select(`smp.shop_member_id, sm.shop_id, s.name AS shop_name,
			sm.user_id, u.email AS user_email,
			TRIM(CONCAT(u.first_name, ' ', u.last_name)) AS user_name,
			sm.role, smp.action, pa.resource, pa.label,
			smp.allow AS override, COALESCE(rad.allow, false) AS default_allow`).
		Joins("JOIN shop_members sm ON sm.id = smp.shop_member_id").
		Joins("JOIN shops s ON s.id = sm.shop_id").
		Joins("JOIN users u ON u.id = sm.user_id").
		Joins("JOIN permission_actions pa ON pa.name = smp.action").
		Joins("LEFT JOIN role_action_defaults rad ON rad.role = sm.role AND rad.action = smp.action").
		Where("smp.deleted_at IS NULL").
		Where("smp.allow IS DISTINCT FROM COALESCE(rad.allow, false)").
		Order("s.name, u.email, smp.action").
		Scan(&out).Error
	return out, err
}

// SetRoleDefault upserts a global role default. role_action_defaults has no
// shop_id — MemberCan resolves the same matrix for every shop that has no
// per-member override for that action — so this change takes effect for
// every shop on the platform the instant it's saved, for every member of
// that role without an explicit override.
func SetRoleDefault(role, action string, allow bool) (models.RoleActionDefault, error) {
	var out models.RoleActionDefault
	var n int64
	initializers.DB.Model(&models.ShopRole{}).Where("name = ?", role).Count(&n)
	if n == 0 {
		return out, ErrInvalidRole
	}
	if !validAction(action) {
		return out, ErrInvalidAction
	}
	out = models.RoleActionDefault{Role: role, Action: action, Allow: allow}
	err := initializers.DB.
		Where("role = ? AND action = ?", role, action).
		Assign(models.RoleActionDefault{Allow: allow}).
		FirstOrCreate(&out).Error
	return out, err
}
