package model

import "time"

// UserSession identifies a login without storing its bearer token.
type UserSession struct {
	ID           string    `json:"id" gorm:"primaryKey;size:36"`
	UserID       uint      `json:"-" gorm:"index"`
	PwdTS        int64     `json:"-"`
	IP           string    `json:"ip" gorm:"size:128"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at" gorm:"index"`
}
