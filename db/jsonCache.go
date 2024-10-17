package db

import (
	"gorm.io/gorm"
	"time"
)

func InsertJSONCache(db *gorm.DB, chatID, jsonContent string) error {
	var count int64
	db.Model(&JSONCache{}).Count(&count)

	if count >= 5 {
		// Find the oldest entry and delete it
		var oldestCache JSONCache
		err := db.Order("last_access asc").First(&oldestCache).Error
		if err != nil {
			return err
		}
		db.Unscoped().Delete(&oldestCache)
	}

	newCache := JSONCache{
		JaiChatID:   chatID,
		JSONContent: jsonContent,
		LastAccess:  time.Now(),
	}
	return db.Create(&newCache).Error
}

func UpdateLastAccess(db *gorm.DB, chatID string) error {
	return db.Model(&JSONCache{}).
		Where("jai_chat_id = ?", chatID).
		Update("last_access", time.Now()).Error
}
