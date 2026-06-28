package db

import (
	"groupbuy/internal/config"
	"groupbuy/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init() error {
	var err error
	DB, err = gorm.Open(sqlite.Open(config.C.Database.DSN), &gorm.Config{})
	if err != nil {
		return err
	}
	return DB.AutoMigrate(
		&model.Leader{},
		&model.Group{},
		&model.Product{},
		&model.Order{},
		&model.DeliveryList{},
		&model.DeliveryItem{},
		&model.Commission{},
	)
}
