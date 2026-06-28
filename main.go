package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"groupbuy/internal/config"
	"groupbuy/internal/db"
	"groupbuy/internal/handler"
	"groupbuy/internal/middleware"
	"groupbuy/internal/scheduler"
)

func main() {
	if err := config.Load("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := db.Init(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	scheduler.Start()
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())
	r.Static("/uploads", "./uploads")
	r.Static("/export", "./export")
	api := r.Group("/api")
	{
		api.POST("/leaders", handler.CreateLeader)
		api.GET("/leaders/login", handler.LeaderLogin)
		api.GET("/groups/open", handler.GetOpenGroups)
	}
	auth := api.Group("")
	auth.Use(middleware.LeaderAuth)
	{
		auth.POST("/groups", handler.CreateGroup)
		auth.GET("/groups", handler.ListGroups)
		auth.GET("/groups/:id", handler.GetGroup)
		auth.POST("/groups/:group_id/products", handler.CreateProduct)
		auth.GET("/groups/:group_id/products", handler.ListProducts)
		auth.POST("/products/:id/image", handler.UploadProductImage)
		auth.POST("/orders", handler.CreateOrder)
		auth.GET("/orders", handler.ListOrders)
		auth.GET("/orders/export", handler.ExportOrders)
		auth.GET("/orders/:id", handler.GetOrder)
		auth.PUT("/orders/:id/cancel", handler.CancelOrder)
		auth.POST("/deliveries/cutoff", handler.CutoffGroups)
		auth.GET("/deliveries", handler.ListDeliveryLists)
		auth.GET("/deliveries/:id", handler.GetDeliveryList)
		auth.PUT("/deliveries/:id/arrive", handler.MarkArrived)
		auth.POST("/deliveries/verify", handler.VerifyPickupCode)
		auth.GET("/commissions", handler.ListCommissions)
		auth.POST("/commissions/calculate", handler.CalculateCommission)
		auth.PUT("/commissions/:id/settle", handler.SettleCommission)
	}
	addr := fmt.Sprintf(":%d", config.C.Server.Port)
	log.Printf("团购后端启动在 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}
