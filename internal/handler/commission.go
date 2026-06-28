package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/config"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
)

func CalculateCommission(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	month := c.Query("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if err := CalculateLeaderCommission(leaderID, month); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "佣金计算完成", "month": month})
}

func CalculateLeaderCommission(leaderID uint, month string) error {
	var leader model.Leader
	if err := db.DB.First(&leader, leaderID).Error; err != nil {
		return fmt.Errorf("团长不存在: %w", err)
	}
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return fmt.Errorf("月份格式错误: %w", err)
	}
	end := start.AddDate(0, 1, 0)
	var totalAmount float64
	var totalOrders int64
	db.DB.Model(&model.Order{}).
		Where("leader_id = ? AND status != ? AND created_at >= ? AND created_at < ?", leaderID, "cancelled", start, end).
		Count(&totalOrders)
	db.DB.Model(&model.Order{}).
		Where("leader_id = ? AND status != ? AND created_at >= ? AND created_at < ?", leaderID, "cancelled", start, end).
		Select("COALESCE(SUM(total_price), 0)").Scan(&totalAmount)
	rate := leader.CommissionRate
	if rate == 0 {
		rate = config.C.Commission.Rate
	}
	commissionAmount := totalAmount * rate
	var existing model.Commission
	isNew := db.DB.Where("leader_id = ? AND month = ?", leaderID, month).First(&existing).Error != nil
	if isNew {
		comm := model.Commission{
			LeaderID:         leaderID,
			Month:            month,
			TotalOrders:      int(totalOrders),
			TotalAmount:      totalAmount,
			CommissionRate:   rate,
			CommissionAmount: commissionAmount,
			Status:           "pending",
		}
		return db.DB.Create(&comm).Error
	}
	return db.DB.Model(&existing).Updates(map[string]interface{}{
		"total_orders":      int(totalOrders),
		"total_amount":      totalAmount,
		"commission_rate":   rate,
		"commission_amount": commissionAmount,
	}).Error
}

func ListCommissions(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	month := c.Query("month")
	status := c.Query("status")
	var commissions []model.Commission
	q := db.DB.Where("leader_id = ?", leaderID)
	if month != "" {
		q = q.Where("month = ?", month)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Preload("Leader").Order("month DESC").Find(&commissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, commissions)
}

func SettleCommission(c *gin.Context) {
	id := c.Param("id")
	leaderID := c.GetUint("leader_id")
	var comm model.Commission
	if err := db.DB.Where("id = ? AND leader_id = ?", id, leaderID).First(&comm).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "佣金记录不存在"})
		return
	}
	if comm.Status == "settled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "佣金已结算"})
		return
	}
	db.DB.Model(&comm).Update("status", "settled")
	c.JSON(http.StatusOK, gin.H{"message": "佣金已结算", "commission": comm})
}
