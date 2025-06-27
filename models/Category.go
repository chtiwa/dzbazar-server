package models

type Category struct {
	BaseModel
	Title    string    `gorm:"not null" json:"title"`
	Products []Product `gorm:"foreignKey:CategoryID;references:ID" json:"products"` // in the Product model, the CategoryID is the foreignKey and the referenced primary key in Category is ID
}
