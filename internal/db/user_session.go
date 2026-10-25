package db

import (
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

func CreateUserSession(session *model.UserSession) error {
	return db.Create(session).Error
}

func GetUserSession(id string) (*model.UserSession, error) {
	var session model.UserSession
	err := db.Where("id = ? AND expires_at > ?", id, time.Now()).Take(&session).Error
	return &session, err
}

func TouchUserSession(id string) {
	now := time.Now()
	db.Model(&model.UserSession{}).Where("id = ? AND last_active_at < ?", id, now.Add(-time.Minute)).Update("last_active_at", now)
}

func ListUserSessions(user *model.User) ([]model.UserSession, error) {
	sessions := make([]model.UserSession, 0)
	err := db.Where("user_id = ? AND pwd_ts = ? AND expires_at > ?", user.ID, user.PwdTS, time.Now()).Order("last_active_at DESC").Find(&sessions).Error
	return sessions, err
}

func DeleteUserSession(userID uint, id string) error {
	return db.Where("user_id = ? AND id = ?", userID, id).Delete(&model.UserSession{}).Error
}

func DeleteOtherUserSessions(userID uint, currentID string) error {
	return db.Where("user_id = ? AND id <> ?", userID, currentID).Delete(&model.UserSession{}).Error
}
