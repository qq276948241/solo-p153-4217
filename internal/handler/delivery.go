package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
	"groupbuy/internal/service"
)

func CutoffGroups(c *gin.Context) {
	count, err := RunCutoff()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "截单完成", "groups_closed": count})
}

func RunCutoff() (int64, error) {
	now := time.Now()
	result := db.DB.Model(&model.Group{}).
		Where("status = ? AND cutoff_at <= ?", "open", now).
		Update("status", "closed")
	if result.Error != nil {
		return 0, result.Error
	}
	var closedGroups []model.Group
	db.DB.Where("status = ? AND cutoff_at <= ?", "closed", now).Find(&closedGroups)
	for _, g := range closedGroups {
		if err := generateDeliveryList(g.ID, g.LeaderID); err != nil {
			log.Printf("生成配送清单失败 group_id=%d: %v", g.ID, err)
		}
	}
	return result.RowsAffected, nil
}

func generateDeliveryList(groupID, leaderID uint) error {
	var existing model.DeliveryList
	if err := db.DB.Where("group_id = ?", groupID).First(&existing).Error; err == nil {
		return nil
	}
	var orders []model.Order
	if err := db.DB.Where("group_id = ? AND status != ?", groupID, "cancelled").Preload("Product").Find(&orders).Error; err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}
	deliveryDate := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dl := model.DeliveryList{
		LeaderID:     leaderID,
		GroupID:      groupID,
		DeliveryDate: deliveryDate,
		Status:       "ready",
	}
	if err := db.DB.Create(&dl).Error; err != nil {
		return err
	}
	var items []model.DeliveryItem
	for _, o := range orders {
		stockRemaining := service.Stock.GetRemaining(o.Product)
		items = append(items, model.DeliveryItem{
			DeliveryListID: dl.ID,
			OrderID:        o.ID,
			ProductName:    o.Product.Name,
			Qty:            o.Qty,
			Stock:          o.Product.Stock,
			StockSold:      o.Product.StockSold,
			StockRemaining: stockRemaining,
			BuyerName:      o.BuyerName,
			BuyerPhone:     o.BuyerPhone,
			PickupPoint:    o.PickupPoint,
			PickupCode:     o.PickupCode,
			Status:         "pending",
		})
		db.DB.Model(&o).Update("status", "arrived")
	}
	return db.DB.Create(&items).Error
}

func ListDeliveryLists(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	date := c.Query("date")
	var lists []model.DeliveryList
	q := db.DB.Where("leader_id = ?", leaderID)
	if date != "" {
		q = q.Where("delivery_date = ?", date)
	}
	if err := q.Preload("Items").Preload("Leader").Order("created_at DESC").Find(&lists).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lists)
}

func GetDeliveryList(c *gin.Context) {
	id := c.Param("id")
	var dl model.DeliveryList
	if err := db.DB.Preload("Items").Preload("Leader").First(&dl, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配送清单不存在"})
		return
	}
	c.JSON(http.StatusOK, dl)
}

func MarkArrived(c *gin.Context) {
	id := c.Param("id")
	leaderID := c.GetUint("leader_id")
	var dl model.DeliveryList
	if err := db.DB.Where("id = ? AND leader_id = ?", id, leaderID).First(&dl).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配送清单不存在"})
		return
	}
	tx := db.DB.Begin()
	if err := tx.Model(&dl).Update("status", "arrived").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Model(&model.DeliveryItem{}).Where("delivery_list_id = ?", dl.ID).Update("status", "arrived").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "已标记为到货"})
}

func VerifyPickupCode(c *gin.Context) {
	var req struct {
		PickupCode string `json:"pickup_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var item model.DeliveryItem
	if err := db.DB.Where("pickup_code = ?", req.PickupCode).First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "提货码无效"})
		return
	}
	if item.Status == "picked_up" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该提货码已核销", "item": item})
		return
	}
	db.DB.Model(&item).Update("status", "picked_up")
	var order model.Order
	if err := db.DB.First(&order, item.OrderID).Error; err == nil {
		db.DB.Model(&order).Update("status", "picked_up")
	}
	c.JSON(http.StatusOK, gin.H{"message": "核销成功", "item": item})
}
