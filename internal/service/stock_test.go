package service_test

import (
	"os"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"groupbuy/internal/model"
	"groupbuy/internal/service"
)

func setupTestDB(t *testing.T) *gorm.DB {
	f, err := os.CreateTemp("", "test_stock_*.db")
	if err != nil {
		t.Fatalf("创建临时数据库文件失败: %v", err)
	}
	f.Close()
	dbPath := f.Name()
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&model.Product{}); err != nil {
		t.Fatalf("迁移表失败: %v", err)
	}
	return db
}

func TestDeductStockConcurrent(t *testing.T) {
	db := setupTestDB(t)
	product := model.Product{
		GroupID: 1,
		Name:    "车厘子JJJ 5斤装",
		Price:   128.00,
		Stock:   1,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("创建测试商品失败: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx := db.Begin()
			err := service.Stock.Deduct(tx, product.ID, 1)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				tx.Rollback()
				return
			}
			tx.Commit()
		}()
	}
	wg.Wait()

	var updated model.Product
	if err := db.First(&updated, product.ID).Error; err != nil {
		t.Fatalf("查询更新后的商品失败: %v", err)
	}

	t.Logf("总请求数: %d, 成功数: %d, 失败数: %d", concurrency, concurrency-errCount, errCount)
	t.Logf("最终 Stock: %d, StockSold: %d, Version: %d", updated.Stock, updated.StockSold, updated.Version)

	if updated.StockSold > updated.Stock {
		t.Errorf("超卖！库存 %d，已售出 %d", updated.Stock, updated.StockSold)
	}
	if updated.StockSold != 1 {
		t.Errorf("期望已售出 1 件，实际 %d 件", updated.StockSold)
	}
	if errCount != concurrency-1 {
		t.Errorf("期望 %d 个失败请求，实际 %d 个", concurrency-1, errCount)
	}
	if updated.Version < 1 {
		t.Errorf("版本号未递增，当前 %d", updated.Version)
	}
}

func TestDeductStockUnlimited(t *testing.T) {
	db := setupTestDB(t)
	product := model.Product{
		GroupID: 1,
		Name:    "散装蔬菜（不限量）",
		Price:   5.00,
		Stock:   0,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("创建测试商品失败: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx := db.Begin()
			err := service.Stock.Deduct(tx, product.ID, 1)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				tx.Rollback()
				return
			}
			tx.Commit()
		}()
	}
	wg.Wait()

	if errCount != 0 {
		t.Errorf("不限量商品不应失败，失败数: %d", errCount)
	}
}

func TestRestoreStockConcurrent(t *testing.T) {
	db := setupTestDB(t)
	product := model.Product{
		GroupID:   1,
		Name:      "排骨 2斤装",
		Price:     45.00,
		Stock:     10,
		StockSold: 5,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("创建测试商品失败: %v", err)
	}

	const concurrency = 5
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx := db.Begin()
			err := service.Stock.Restore(tx, product.ID, 1)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				tx.Rollback()
				return
			}
			tx.Commit()
		}()
	}
	wg.Wait()

	var updated model.Product
	if err := db.First(&updated, product.ID).Error; err != nil {
		t.Fatalf("查询更新后的商品失败: %v", err)
	}

	t.Logf("总请求数: %d, 成功数: %d, 失败数: %d", concurrency, concurrency-errCount, errCount)
	t.Logf("最终 Stock: %d, StockSold: %d, Version: %d", updated.Stock, updated.StockSold, updated.Version)

	expectedSold := 5 - concurrency
	if expectedSold < 0 {
		expectedSold = 0
	}
	if updated.StockSold != expectedSold {
		t.Errorf("期望 StockSold = %d，实际 %d", expectedSold, updated.StockSold)
	}
}

func TestDeductStockExhausted(t *testing.T) {
	db := setupTestDB(t)
	product := model.Product{
		GroupID:   1,
		Name:      "售罄商品",
		Price:     10.00,
		Stock:     3,
		StockSold: 3,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("创建测试商品失败: %v", err)
	}

	tx := db.Begin()
	err := service.Stock.Deduct(tx, product.ID, 1)
	if err == nil {
		t.Error("售罄商品应该返回错误，实际成功")
		tx.Rollback()
	} else {
		t.Logf("正确返回错误: %v", err)
		tx.Rollback()
	}
}
