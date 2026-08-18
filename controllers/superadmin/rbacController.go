package superadmin

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
)

// GetPermissionCatalog returns every gate-able action plus the
// owner/moderator/confirmation role defaults for it. Global, non-shop-scoped
// data — mounted under supportAccessible in routes.
func GetPermissionCatalog(c *gin.Context) {
	catalog, err := services.GetPermissionCatalog()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch permission catalog", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": catalog})
}

// ListRoleDeviations returns every shop-member-level override that currently
// disagrees with its member's role default — the "why does this employee
// have unusual access" diagnostic view for support/ops.
func ListRoleDeviations(c *gin.Context) {
	deviations, err := services.ListRoleDeviations()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch permission overrides", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": deviations})
}

type UpdateRoleDefaultInput struct {
	Allow bool `json:"allow"`
}

// UpdateRoleDefault toggles a global role default (owner/moderator/
// confirmation x action). This is platform-wide config, not shop-owned data,
// but it takes effect for every shop immediately, so it stays super_admin-only.
func UpdateRoleDefault(c *gin.Context) {
	role := c.Param("role")
	action := c.Param("action")

	var body UpdateRoleDefaultInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	def, err := services.SetRoleDefault(role, action, body.Allow)
	if err != nil {
		if errors.Is(err, services.ErrInvalidRole) || errors.Is(err, services.ErrInvalidAction) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Unknown role or action"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to update role default", err)
		return
	}

	utils.LogAudit(c, "rbac.role_default.update", "RoleActionDefault", nil, gin.H{"role": role, "action": action, "allow": body.Allow})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Role default updated", "data": def})
}
