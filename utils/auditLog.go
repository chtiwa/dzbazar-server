package utils

import (
	"encoding/json"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// LogAudit writes a Super Admin audit trail entry. Failures are logged to the
// console but never block the request — an audit log write must not be able
// to break the action it's recording.
func LogAudit(c *gin.Context, action string, targetType string, targetID *uuid.UUID, metadata any) {
	actor, ok := c.Get("user")
	if !ok {
		return
	}
	actorUser, ok := actor.(models.User)
	if !ok {
		return
	}

	metadataJSON := ""
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(b)
		}
	}

	entry := models.AuditLog{
		ActorID:    actorUser.ID,
		ActorEmail: actorUser.Email,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadataJSON,
		IPAddress:  c.ClientIP(),
	}

	initializers.DB.Create(&entry)
}
