package handles

import (
	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
)

func ListMySessions(c *gin.Context) {
	user := c.Request.Context().Value(conf.UserKey).(*model.User)
	sessions, err := db.ListUserSessions(user)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	currentID := ""
	if claims, err := common.ParseToken(c.GetHeader("Authorization")); err == nil {
		currentID = claims.ID
	}
	common.SuccessResp(c, gin.H{"sessions": sessions, "current_id": currentID})
}

func DeleteMySession(c *gin.Context) {
	var req struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.Request.Context().Value(conf.UserKey).(*model.User)
	if err := db.DeleteUserSession(user.ID, req.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func DeleteOtherSessions(c *gin.Context) {
	claims, err := common.ParseToken(c.GetHeader("Authorization"))
	if err != nil {
		common.ErrorStrResp(c, "A login session is required", 403)
		return
	}
	user := c.Request.Context().Value(conf.UserKey).(*model.User)
	if err := db.DeleteOtherUserSessions(user.ID, claims.ID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}
