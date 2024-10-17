package db

import (
	"github.com/sashabaranov/go-openai"
	"gorm.io/gorm"
	"time"
)

func StartChat(db *gorm.DB, userID, JSON, fileLocation, initialMessage string) (*JaiChat, error) {
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

	chatMessage := ChatMessages{
		JaiChatID: jaiChat.UUID.ID,
		Role:      openai.ChatMessageRoleAssistant,
		Message:   initialMessage,
	}
	err = db.Create(&chatMessage).Error
	if err != nil {
		return nil, err
	}

	return &jaiChat, nil
}
