package db

import (
	"gorm.io/gorm"
	"time"
)

func StartChat(db *gorm.DB, userID, JSON, fileLocation string) (*JaiChat, error) {
	jaiChat := JaiChat{
		UserID:       userID,
		JSON:         JSON,
		FileLocation: fileLocation,
		Model: gorm.Model{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	err := db.Create(&jaiChat).Error
	if err != nil {
		return nil, err
	}

	return &jaiChat, nil
}
