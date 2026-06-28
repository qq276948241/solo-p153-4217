package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
)

func LeaderAuth(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少授权信息"})
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的授权格式"})
		return
	}
	var leader model.Leader
	if err := db.DB.First(&leader, "phone = ?", token).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "团长不存在"})
		return
	}
	c.Set("leader_id", leader.ID)
	c.Set("leader", leader)
	c.Next()
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
