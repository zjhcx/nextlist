package common

import (
	"time"

	"errors"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

var SecretKey []byte

type UserClaims struct {
	Username string `json:"username"`
	PwdTS    int64  `json:"pwd_ts"`
	jwt.RegisteredClaims
}

func GenerateToken(user *model.User, contexts ...*gin.Context) (tokenString string, err error) {
	claim := UserClaims{
		Username: user.Username,
		PwdTS:    user.PwdTS,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(conf.Conf.TokenExpiresIn) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	tokenString, err = token.SignedString(SecretKey)
	if err != nil {
		return "", err
	}
	session := model.UserSession{ID: claim.ID, UserID: user.ID, PwdTS: user.PwdTS,
		CreatedAt: time.Now(), LastActiveAt: time.Now(), ExpiresAt: claim.ExpiresAt.Time}
	if len(contexts) > 0 && contexts[0] != nil {
		session.IP = contexts[0].ClientIP()
		session.UserAgent = contexts[0].Request.UserAgent()
	}
	if err = db.CreateUserSession(&session); err != nil {
		return "", err
	}
	return tokenString, err
}

func ParseToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		return SecretKey, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok {
			if ve.Errors&jwt.ValidationErrorMalformed != 0 {
				return nil, errors.New("that's not even a token")
			} else if ve.Errors&jwt.ValidationErrorExpired != 0 {
				return nil, errors.New("token is expired")
			} else if ve.Errors&jwt.ValidationErrorNotValidYet != 0 {
				return nil, errors.New("token not active yet")
			} else {
				return nil, errors.New("couldn't handle this token")
			}
		}
	}
	if token == nil {
		return nil, errors.New("invalid token")
	}
	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		session, err := db.GetUserSession(claims.ID)
		if err != nil || session.PwdTS != claims.PwdTS {
			return nil, errors.New("token is invalidated")
		}
		db.TouchUserSession(claims.ID)
		return claims, nil
	}
	return nil, errors.New("couldn't handle this token")
}

func InvalidateToken(tokenString string) error {
	if tokenString == "" {
		return nil // don't invalidate empty guest token
	}
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil
	}
	session, err := db.GetUserSession(claims.ID)
	if err != nil {
		return err
	}
	return db.DeleteUserSession(session.UserID, claims.ID)
}

func IsTokenInvalidated(tokenString string) bool {
	_, err := ParseToken(tokenString)
	return err != nil
}
