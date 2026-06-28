package service

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"groupbuy/internal/model"
)

var (
	ErrStockUnavailable = errors.New("库存不足，该团品已售罄")
	ErrStockDeductFail  = errors.New("扣减库存失败")
	ErrStockRestoreFail = errors.New("回滚库存失败")
	ErrProductNotFound  = errors.New("团品不存在")
	ErrVersionConflict  = errors.New("版本冲突，请重试")
)

type StockService struct{}

var Stock = &StockService{}

const maxRetries = 5

func (s *StockService) CheckAvailable(db *gorm.DB, productID uint, qty int) error {
	var product model.Product
	if err := db.First(&product, productID).Error; err != nil {
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
	for i := 0; i < maxRetries; i++ {
		var product model.Product
		if err := tx.First(&product, productID).Error; err != nil {
			return ErrProductNotFound
		}
		if product.Stock <= 0 {
			return nil
		}
		if product.Stock-product.StockSold < qty {
			return ErrStockUnavailable
		}
		result := tx.Model(&model.Product{}).
			Where("id = ? AND version = ?", productID, product.Version).
			Updates(map[string]interface{}{
				"stock_sold": gorm.Expr("stock_sold + ?", qty),
				"version":    gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return ErrStockDeductFail
		}
		if result.RowsAffected == 1 {
			return nil
		}
		time.Sleep(time.Duration(i*20) * time.Millisecond)
	}
	return ErrStockUnavailable
}

func (s *StockService) Restore(tx *gorm.DB, productID uint, qty int) error {
	for i := 0; i < maxRetries; i++ {
		var product model.Product
		if err := tx.First(&product, productID).Error; err != nil {
			return ErrProductNotFound
		}
		if product.Stock <= 0 {
			return nil
		}
		newSold := product.StockSold - qty
		if newSold < 0 {
			newSold = 0
		}
		result := tx.Model(&model.Product{}).
			Where("id = ? AND version = ?", productID, product.Version).
			Updates(map[string]interface{}{
				"stock_sold": newSold,
				"version":    gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return ErrStockRestoreFail
		}
		if result.RowsAffected == 1 {
			return nil
		}
		time.Sleep(time.Duration(i*20) * time.Millisecond)
	}
	return ErrStockRestoreFail
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
