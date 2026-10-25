package common

import (
	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestSessionTokensAreUniqueAndRevocable(t *testing.T) {
	oldConf, oldKey := conf.Conf, SecretKey
	conf.Conf = &conf.Config{TokenExpiresIn: 1}
	SecretKey = []byte("session-test-secret")
	t.Cleanup(func() { conf.Conf, SecretKey = oldConf, oldKey })
	d, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(d)
	user := &model.User{ID: 1, Username: "test", PwdTS: 1}
	first, err := GenerateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("separate logins share a token")
	}
	claims, err := ParseToken(first)
	if err != nil || claims.ID == "" {
		t.Fatalf("invalid session claims: %v", err)
	}
	if err := InvalidateToken(first); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(first); err == nil {
		t.Fatal("revoked token accepted")
	}
	if _, err := ParseToken(second); err != nil {
		t.Fatal("other login was revoked")
	}
	if _, err := ParseToken("malformed"); err == nil {
		t.Fatal("malformed token accepted")
	}
}
