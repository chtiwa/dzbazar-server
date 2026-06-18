package models

import "github.com/google/uuid"

type SupportTicket struct {
	BaseModel
	ShopID *uuid.UUID `gorm:"type:uuid;index" json:"shopId"`
	Shop   *Shop      `gorm:"foreignKey:ShopID;references:ID" json:"shop,omitempty"`

	RequesterUserID uuid.UUID `gorm:"type:uuid;not null;index" json:"requesterUserId"`
	Requester       User      `gorm:"foreignKey:RequesterUserID;references:ID" json:"requester"`

	Subject  string `gorm:"not null" json:"subject"`
	Status   string `gorm:"default:open;index" json:"status"` // open | pending | resolved | closed
	Priority string `gorm:"default:normal" json:"priority"`   // low | normal | high | urgent

	AssignedToUserID *uuid.UUID `gorm:"type:uuid;index" json:"assignedToUserId"`
	AssignedTo       *User      `gorm:"foreignKey:AssignedToUserID;references:ID" json:"assignedTo,omitempty"`

	Messages []SupportTicketMessage `gorm:"foreignKey:TicketID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
}

type SupportTicketMessage struct {
	BaseModel
	TicketID uuid.UUID `gorm:"type:uuid;not null;index" json:"ticketId"`

	AuthorUserID uuid.UUID `gorm:"type:uuid;not null" json:"authorUserId"`
	Author       User      `gorm:"foreignKey:AuthorUserID;references:ID" json:"author"`

	Body           string `gorm:"not null" json:"body"`
	IsInternalNote bool   `gorm:"default:false" json:"isInternalNote"`
}
