package model

import (
	"time"

	"gorm.io/gorm"
)

type Leader struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	Name           string         `gorm:"size:50;not null" json:"name"`
	Phone          string         `gorm:"size:20;not null;uniqueIndex" json:"phone"`
	Wechat         string         `gorm:"size:50" json:"wechat"`
	CommissionRate float64        `gorm:"default:0.1" json:"commission_rate"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type Group struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	LeaderID  uint           `gorm:"index;not null" json:"leader_id"`
	Title     string         `gorm:"size:200;not null" json:"title"`
	Status    string         `gorm:"size:20;default:open" json:"status"`
	CutoffAt  *time.Time     `json:"cutoff_at"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Leader    Leader         `gorm:"foreignKey:LeaderID" json:"leader,omitempty"`
	Products  []Product      `gorm:"foreignKey:GroupID" json:"products,omitempty"`
}

type ProductSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Product struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	GroupID        uint           `gorm:"index;not null" json:"group_id"`
	Name           string         `gorm:"size:200;not null" json:"name"`
	Image          string         `gorm:"size:500" json:"image"`
	Specs          string         `gorm:"type:text" json:"specs"`
	Price          float64        `gorm:"not null" json:"price"`
	Stock          int            `gorm:"default:0" json:"stock"`
	StockSold      int            `gorm:"default:0" json:"stock_sold"`
	Version        int            `gorm:"default:0" json:"version"`
	StockRemaining int            `gorm:"-" json:"stock_remaining"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Group          Group          `gorm:"foreignKey:GroupID" json:"-"`
}

func (p *Product) AfterFind(tx *gorm.DB) error {
	if p.Stock > 0 {
		p.StockRemaining = p.Stock - p.StockSold
		if p.StockRemaining < 0 {
			p.StockRemaining = 0
		}
	} else {
		p.StockRemaining = -1
	}
	return nil
}

type Order struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	GroupID     uint           `gorm:"index;not null" json:"group_id"`
	ProductID   uint           `gorm:"index;not null" json:"product_id"`
	LeaderID    uint           `gorm:"index;not null" json:"leader_id"`
	BuyerName   string         `gorm:"size:50;not null" json:"buyer_name"`
	BuyerPhone  string         `gorm:"size:20;not null" json:"buyer_phone"`
	PickupPoint string         `gorm:"size:200;not null" json:"pickup_point"`
	Qty         int            `gorm:"not null" json:"qty"`
	TotalPrice  float64        `gorm:"not null" json:"total_price"`
	PickupCode  string         `gorm:"size:20;index" json:"pickup_code"`
	Status      string         `gorm:"size:20;default:pending;index" json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Group       Group          `gorm:"foreignKey:GroupID" json:"-"`
	Product     Product        `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Leader      Leader         `gorm:"foreignKey:LeaderID" json:"-"`
}

type DeliveryList struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	LeaderID     uint           `gorm:"index;not null" json:"leader_id"`
	GroupID      uint           `gorm:"index;not null" json:"group_id"`
	DeliveryDate string         `gorm:"size:20;index" json:"delivery_date"`
	Status       string         `gorm:"size:20;default:ready" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Leader       Leader         `gorm:"foreignKey:LeaderID" json:"-"`
	Items        []DeliveryItem `gorm:"foreignKey:DeliveryListID" json:"items,omitempty"`
}

type DeliveryItem struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	DeliveryListID uint      `gorm:"index;not null" json:"delivery_list_id"`
	OrderID        uint      `gorm:"index;not null" json:"order_id"`
	ProductName    string    `gorm:"size:200;not null" json:"product_name"`
	Qty            int       `gorm:"not null" json:"qty"`
	Stock          int       `gorm:"default:0" json:"stock"`
	StockSold      int       `gorm:"default:0" json:"stock_sold"`
	StockRemaining int       `gorm:"default:0" json:"stock_remaining"`
	BuyerName      string    `gorm:"size:50;not null" json:"buyer_name"`
	BuyerPhone     string    `gorm:"size:20;not null" json:"buyer_phone"`
	PickupPoint    string    `gorm:"size:200;not null" json:"pickup_point"`
	PickupCode     string    `gorm:"size:20;index" json:"pickup_code"`
	Status         string    `gorm:"size:20;default:pending" json:"status"`
}

type Commission struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	LeaderID          uint      `gorm:"index;not null" json:"leader_id"`
	Month             string    `gorm:"size:10;index" json:"month"`
	TotalOrders       int       `json:"total_orders"`
	TotalAmount       float64   `json:"total_amount"`
	CommissionRate    float64   `json:"commission_rate"`
	CommissionAmount  float64   `json:"commission_amount"`
	Status            string    `gorm:"size:20;default:pending" json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Leader            Leader    `gorm:"foreignKey:LeaderID" json:"-"`
}
