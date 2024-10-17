package db

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

var Tables = []interface{}{
	&User{},
	&JaiChat{},
	&ChatMessages{},
}

type UUID struct {
	ID string `gorm:"type:varchar(40);not null;unique_index"`
}

func (id *UUID) BeforeCreate(tx *gorm.DB) (err error) {
	id.ID = uuid.New().String()
	return
}

type User struct {
	UUID
	Name             string
	Email            string    `gorm:"not null;unique"`
	Pin              string    `gorm:"not null"`
	TokensUsed       int       `gorm:"default:0"`
	TokenLastRefresh time.Time `gorm:"default:now()"`
	gorm.Model
}

type JaiChat struct {
	UUID
	UserID       string `gorm:"not null"`
	JSON         string `gorm:"not null"`
	FileLocation string `gorm:"not null"`
	gorm.Model
	User User `gorm:"foreignkey:UserID"`
}

type ChatMessages struct {
	JaiChatID string `gorm:"not null"`
	Role      string `gorm:"not null"`
	Message   string `gorm:"not null"`
	gorm.Model
	JaiChat JaiChat `gorm:"foreignkey:JaiChatID"`
}
