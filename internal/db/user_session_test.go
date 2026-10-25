package db

import (
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"testing"
	"time"
)

func TestUserSessionIsolationAndRevocation(t *testing.T) {
	old := db
	db = openTestDB(t, "session_test_")
	t.Cleanup(func() { db = old })
	if err := db.AutoMigrate(&model.UserSession{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, session := range []model.UserSession{
		{ID: "current", UserID: 1, PwdTS: 10, ExpiresAt: now.Add(time.Hour)},
		{ID: "other", UserID: 1, PwdTS: 10, ExpiresAt: now.Add(time.Hour)},
		{ID: "foreign", UserID: 2, PwdTS: 10, ExpiresAt: now.Add(time.Hour)},
		{ID: "expired", UserID: 1, PwdTS: 10, ExpiresAt: now.Add(-time.Hour)},
		{ID: "old-password", UserID: 1, PwdTS: 9, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := CreateUserSession(&session); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := ListUserSessions(&model.User{ID: 1, PwdTS: 10})
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions: %v, %v", sessions, err)
	}
	if _, err := GetUserSession("expired"); err == nil {
		t.Fatal("accepted expired session")
	}
	if err := DeleteUserSession(1, "foreign"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetUserSession("foreign"); err != nil {
		t.Fatal("deleted another user's session")
	}
	if err := DeleteOtherUserSessions(1, "current"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetUserSession("current"); err != nil {
		t.Fatal("deleted current session")
	}
	if _, err := GetUserSession("other"); err == nil {
		t.Fatal("other session still active")
	}
	if err := DeleteUserSession(1, "current"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetUserSession("current"); err == nil {
		t.Fatal("revoked session still active")
	}
}
