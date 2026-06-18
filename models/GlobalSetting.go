package models

// GlobalSetting is a simple platform-wide key/value store (e.g. maintenance
// mode, support email, default currency). ValueType is advisory metadata for
// the frontend to render the right input control.
type GlobalSetting struct {
	BaseModel
	Key         string `gorm:"uniqueIndex;not null" json:"key"`
	Value       string `json:"value"`
	ValueType   string `gorm:"default:string" json:"valueType"` // string | number | bool | json
	Description string `json:"description"`
}
