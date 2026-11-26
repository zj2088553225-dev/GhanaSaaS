package models

type PickupStationStatus string

const (
	PickupStationStatusActive   PickupStationStatus = "active"
	PickupStationStatusDisabled PickupStationStatus = "disabled"
)

type PickupStation struct {
	BaseModel
	Name      string              `gorm:"type:varchar(120);not null" json:"name"`
	Address   string              `gorm:"type:text;not null" json:"address"`
	ManagerID uint                `gorm:"not null;index" json:"manager_id"`
	Manager   User                `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"manager,omitempty"`
	Status    PickupStationStatus `gorm:"type:varchar(32);not null;index" json:"status"`
}
