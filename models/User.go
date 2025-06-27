package models

type User struct {
	BaseModel
	Username string `gorm:"unique" json:"username"`
	Password string `json:"password"`
	Role     string `gorm:"default:User" json:"role" binding:"oneof=Admin User"`
}
