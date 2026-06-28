package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/config"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
)

type CreateGroupReq struct {
	Title    string     `json:"title" binding:"required"`
	CutoffAt *time.Time `json:"cutoff_at"`
}

func CreateGroup(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	var req CreateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cutoffAt := req.CutoffAt
	if cutoffAt == nil {
		tomorrow := time.Now().AddDate(0, 0, 1)
		defaultCutoff := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), config.C.Scheduler.CutoffHour, 0, 0, 0, tomorrow.Location())
		cutoffAt = &defaultCutoff
	}
	group := model.Group{
		LeaderID: leaderID,
		Title:    req.Title,
		Status:   "open",
		CutoffAt: cutoffAt,
	}
	if err := db.DB.Create(&group).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, group)
}

func ListGroups(c *gin.Context) {
	leaderID := c.GetUint("leader_id")
	status := c.Query("status")
	var groups []model.Group
	q := db.DB.Where("leader_id = ?", leaderID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("created_at DESC").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func GetGroup(c *gin.Context) {
	id := c.Param("id")
	var group model.Group
	if err := db.DB.Preload("Products").Preload("Leader").First(&group, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团期不存在"})
		return
	}
	c.JSON(http.StatusOK, group)
}

type CreateProductReq struct {
	Name  string              `json:"name" binding:"required"`
	Specs []model.ProductSpec `json:"specs"`
	Price float64             `json:"price" binding:"required,gt=0"`
	Stock int                 `json:"stock"`
}

func CreateProduct(c *gin.Context) {
	groupID := c.Param("group_id")
	var group model.Group
	if err := db.DB.First(&group, groupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团期不存在"})
		return
	}
	if group.Status != "open" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "团期已截单，无法添加团品"})
		return
	}
	var req CreateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	specsJSON := "[]"
	if len(req.Specs) > 0 {
		b, err := json.Marshal(req.Specs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "规格序列化失败"})
			return
		}
		specsJSON = string(b)
	}
	product := model.Product{
		GroupID: group.ID,
		Name:    req.Name,
		Specs:   specsJSON,
		Price:   req.Price,
		Stock:   req.Stock,
	}
	if err := db.DB.Create(&product).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, product)
}

func UploadProductImage(c *gin.Context) {
	productID := c.Param("id")
	var product model.Product
	if err := db.DB.First(&product, productID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团品不存在"})
		return
	}
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传图片文件"})
		return
	}
	defer file.Close()
	maxBytes := int64(config.C.Upload.MaxSizeMB) * 1024 * 1024
	if header.Size > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("图片大小不能超过%dMB", config.C.Upload.MaxSizeMB)})
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 jpg/png/webp 格式"})
		return
	}
	filename := fmt.Sprintf("%d_%d%s", product.ID, time.Now().UnixMilli(), ext)
	dst := filepath.Join(config.C.Upload.Dir, filename)
	out, err := os.Create(dst)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存图片失败"})
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入图片失败"})
		return
	}
	imageURL := "/uploads/" + filename
	db.DB.Model(&product).Update("image", imageURL)
	c.JSON(http.StatusOK, gin.H{"image": imageURL})
}

func ListProducts(c *gin.Context) {
	groupID := c.Param("group_id")
	var products []model.Product
	if err := db.DB.Where("group_id = ?", groupID).Order("created_at DESC").Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, products)
}

func CreateLeader(c *gin.Context) {
	var req struct {
		Name   string  `json:"name" binding:"required"`
		Phone  string  `json:"phone" binding:"required"`
		Wechat string  `json:"wechat"`
		Rate   float64 `json:"commission_rate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rate := config.C.Commission.Rate
	if req.Rate > 0 {
		rate = req.Rate
	}
	leader := model.Leader{
		Name:           req.Name,
		Phone:          req.Phone,
		Wechat:         req.Wechat,
		CommissionRate: rate,
	}
	if err := db.DB.Create(&leader).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, leader)
}

func LeaderLogin(c *gin.Context) {
	phone := c.Query("phone")
	var leader model.Leader
	if err := db.DB.Where("phone = ?", phone).First(&leader).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "团长不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": leader.Phone, "leader": leader})
}

func GetOpenGroups(c *gin.Context) {
	var groups []model.Group
	if err := db.DB.Where("status = ?", "open").Preload("Products").Preload("Leader").Order("created_at DESC").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func generatePickupCode() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%06d", r.Intn(1000000))
}

func parseID(s string) uint {
	n, _ := strconv.ParseUint(s, 10, 32)
	return uint(n)
}
