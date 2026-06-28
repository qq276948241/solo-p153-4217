package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/config"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
	"groupbuy/internal/service"
)

type CreateOrderReq struct {
	GroupID     uint   `json:"group_id" binding:"required"`
	ProductID   uint   `json:"product_id" binding:"required"`
	BuyerName   string `json:"buyer_name" binding:"required"`
	BuyerPhone  string `json:"buyer_phone" binding:"required"`
	PickupPoint string `json:"pickup_point" binding:"required"`
	Qty         int    `json:"qty" binding:"required,gt=0"`
}

func CreateOrder(c *gin.Context) {
	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var group model.Group
	if err := db.DB.First(&group, req.GroupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团期不存在"})
		return
	}
	if group.Status != "open" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "团期已截单，无法下单"})
		return
	}
	if group.CutoffAt != nil && time.Now().After(*group.CutoffAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "已过截单时间"})
		return
	}
	var product model.Product
	if err := db.DB.First(&product, req.ProductID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团品不存在"})
		return
	}
	if product.GroupID != req.GroupID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "团品不属于该团期"})
		return
	}
	if err := service.Stock.CheckAvailable(product.ID, req.Qty); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := service.Stock.Deduct(tx, product.ID, req.Qty); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	totalPrice := product.Price * float64(req.Qty)
	pickupCode := generatePickupCode()
	for {
		var count int64
		db.DB.Model(&model.Order{}).Where("pickup_code = ?", pickupCode).Count(&count)
		if count == 0 {
			break
		}
		pickupCode = generatePickupCode()
	}
	order := model.Order{
		GroupID:     req.GroupID,
		ProductID:   req.ProductID,
		LeaderID:    group.LeaderID,
		BuyerName:   req.BuyerName,
		BuyerPhone:  req.BuyerPhone,
		PickupPoint: req.PickupPoint,
		Qty:         req.Qty,
		TotalPrice:  totalPrice,
		PickupCode:  pickupCode,
		Status:      "pending",
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交订单失败"})
		return
	}
	db.DB.Preload("Product").First(&order, order.ID)
	c.JSON(http.StatusCreated, order)
}

func ListOrders(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	date := c.Query("date")
	groupID := c.Query("group_id")
	status := c.Query("status")
	var orders []model.Order
	q := db.DB.Where("leader_id = ?", leaderID)
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			nextDay := t.AddDate(0, 0, 1)
			q = q.Where("created_at >= ? AND created_at < ?", t, nextDay)
		}
	}
	if groupID != "" {
		q = q.Where("group_id = ?", groupID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Preload("Product").Order("created_at DESC").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, orders)
}

func GetOrder(c *gin.Context) {
	id := c.Param("id")
	var order model.Order
	if err := db.DB.Preload("Product").Preload("Group").First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	c.JSON(http.StatusOK, order)
}

func CancelOrder(c *gin.Context) {
	id := c.Param("id")
	leaderID := c.GetUint("leader_id")
	var order model.Order
	if err := db.DB.Where("id = ? AND leader_id = ?", id, leaderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	if order.Status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订单已取消"})
		return
	}

	tx := db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Model(&order).Update("status", "cancelled").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消订单失败"})
		return
	}

	if err := service.Stock.Restore(tx, order.ProductID, order.Qty); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交失败"})
		return
	}
	db.DB.Preload("Product").First(&order, id)
	c.JSON(http.StatusOK, order)
}

func ExportOrders(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	date := c.Query("date")
	groupID := c.Query("group_id")
	var orders []model.Order
	q := db.DB.Where("leader_id = ?", leaderID)
	if date != "" {
		t, err := time.Parse("2006-01-02", date)
		if err == nil {
			nextDay := t.AddDate(0, 0, 1)
			q = q.Where("created_at >= ? AND created_at < ?", t, nextDay)
		}
	}
	if groupID != "" {
		q = q.Where("group_id = ?", groupID)
	}
	if err := q.Preload("Product").Order("created_at DESC").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("orders_%d_%s.csv", leaderID, time.Now().Format("20060102150405"))
	filepath := filepath.Join(config.C.Export.Dir, filename)
	f, err := os.Create(filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建导出文件失败"})
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"订单ID", "团品名称", "总库存", "已售出", "剩余库存", "购买人", "手机号", "自提点", "数量", "总价", "提货码", "状态", "下单时间"})
	for _, o := range orders {
		stock := "不限"
		stockSold := "-"
		stockRemaining := "不限"
		remaining := service.Stock.GetRemaining(o.Product)
		if remaining >= 0 {
			stock = fmt.Sprintf("%d", o.Product.Stock)
			stockSold = fmt.Sprintf("%d", o.Product.StockSold)
			stockRemaining = fmt.Sprintf("%d", remaining)
		}
		w.Write([]string{
			fmt.Sprintf("%d", o.ID),
			o.Product.Name,
			stock,
			stockSold,
			stockRemaining,
			o.BuyerName,
			o.BuyerPhone,
			o.PickupPoint,
			fmt.Sprintf("%d", o.Qty),
			fmt.Sprintf("%.2f", o.TotalPrice),
			o.PickupCode,
			statusLabel(o.Status),
			o.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"file": "/export/" + filename, "count": len(orders)})
}

func statusLabel(s string) string {
	switch s {
	case "pending":
		return "待处理"
	case "paid":
		return "已付款"
	case "arrived":
		return "已到货"
	case "picked_up":
		return "已提货"
	case "cancelled":
		return "已取消"
	default:
		return s
	}
}
