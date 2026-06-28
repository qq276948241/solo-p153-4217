package service

import (
	"errors"

	"gorm.io/gorm"
	"groupbuy/internal/db"
	"groupbuy/internal/model"
)

var (
	ErrStockUnavailable = errors.New("库存不足，该团品已售罄")
	ErrStockDeductFail  = errors.New("扣减库存失败")
	ErrStockRestoreFail = errors.New("回滚库存失败")
	ErrProductNotFound  = errors.New("团品不存在")
)

type StockService struct{}

var Stock = &StockService{}

func (s *StockService) CheckAvailable(productID uint, qty int) error {
	var product model.Product
	if err := db.DB.First(&product, productID).Error; err != nil {
		return ErrProductNotFound
	}
	if product.Stock <= 0 {
		return nil
	}
	remaining := product.Stock - product.StockSold
	if remaining < qty {
		return ErrStockUnavailable
	}
	return nil
}

func (s *StockService) Deduct(tx *gorm.DB, productID uint, qty int) error {
	if tx == nil {
		tx = db.DB
	}
	var product model.Product
	if err := tx.First(&product, productID).Error; err != nil {
		return ErrProductNotFound
	}
	if product.Stock <= 0 {
		return nil
	}
	result := tx.Model(&model.Product{}).
		Where("id = ? AND stock_sold + ? <= stock", productID, qty).
		Update("stock_sold", gorm.Expr("stock_sold + ?", qty))
	if result.Error != nil {
		return ErrStockDeductFail
	}
	if result.RowsAffected == 0 {
		return ErrStockUnavailable
	}
	return nil
}

func (s *StockService) Restore(tx *gorm.DB, productID uint, qty int) error {
	if tx == nil {
		tx = db.DB
	}
	var product model.Product
	if err := tx.First(&product, productID).Error; err != nil {
		return ErrProductNotFound
	}
	if product.Stock <= 0 {
		return nil
	}
	result := tx.Model(&model.Product{}).
		Where("id = ? AND stock_sold >= ?", productID, qty).
		Update("stock_sold", gorm.Expr("stock_sold - ?", qty))
	if result.Error != nil {
		return ErrStockRestoreFail
	}
	if result.RowsAffected == 0 {
		if err := tx.Model(&model.Product{}).
			Where("id = ?", productID).
			Update("stock_sold", 0).Error; err != nil {
			return ErrStockRestoreFail
		}
	}
	return nil
}

func (s *StockService) GetRemaining(product model.Product) int {
	if product.Stock <= 0 {
		return -1
	}
	remaining := product.Stock - product.StockSold
	if remaining < 0 {
		return 0
	}
	return remaining
}
