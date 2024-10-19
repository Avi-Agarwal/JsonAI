package db

import (
	"github.com/sashabaranov/go-openai"
	"gorm.io/gorm"
	"time"
)

func StartChat(db *gorm.DB, userID, JSON, fileLocation, initialMessage string, tokenEstimate int) (*JaiChat, error) {
	jaiChat := JaiChat{
		UserID:            userID,
		JSON:              JSON,
		FileLocation:      fileLocation,
		FileTokenEstimate: tokenEstimate,
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

func GetUserChatCount(db *gorm.DB, userID string) (int64, error) {
	var count int64
	err := db.Model(&JaiChat{}).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func GetChatByID(db *gorm.DB, chatID string) (*JaiChat, []*ChatMessages, error) {
	var chat JaiChat
	err := db.Where("id = ?", chatID).First(&chat).Error
	if err != nil {
		return nil, nil, err
	}

	var messages []*ChatMessages
	err = db.Where("jai_chat_id = ?", chatID).Order("created_at ASC").Find(&messages).Error
	if err != nil {
		return nil, nil, err
	}

	return &chat, messages, nil
}

func GetChatsByUserID(db *gorm.DB, userID string) ([]*JaiChat, error) {
	var chats []*JaiChat
	err := db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&chats).Error
	if err != nil {
		return nil, err
	}
	return chats, nil
}

func GetChatMessageCount(db *gorm.DB, chatID string) (int64, error) {
	var count int64
	err := db.Model(&ChatMessages{}).Where("jai_chat_id = ?", chatID).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func AddChatMessage(db *gorm.DB, message *ChatMessages) error {
	err := db.Create(&message).Error
	return err
}
