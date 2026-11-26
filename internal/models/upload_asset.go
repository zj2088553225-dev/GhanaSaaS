package models

type UploadAsset struct {
	BaseModel
	Hash      string `gorm:"size:64;not null;uniqueIndex:idx_upload_asset_type_hash" json:"hash"`
	URL       string `gorm:"size:512;not null" json:"url"`
	FileName  string `gorm:"size:255;not null" json:"file_name"`
	MimeType  string `gorm:"size:100;not null" json:"mime_type"`
	FileSize  int64  `gorm:"not null" json:"file_size"`
	AssetType string `gorm:"size:50;not null;uniqueIndex:idx_upload_asset_type_hash" json:"asset_type"`
}
