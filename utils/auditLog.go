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

// LogAuditSystem writes an audit trail entry with no HTTP-request actor, for
// background jobs (carrier status-sync tickers) that have no gin.Context.
func LogAuditSystem(action string, targetType string, targetID *uuid.UUID, metadata any) {
	metadataJSON := ""
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metadataJSON = string(b)
		}
	}

	entry := models.AuditLog{
		ActorEmail: "system",
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadataJSON,
	}

	initializers.DB.Create(&entry)
}
