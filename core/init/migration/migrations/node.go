package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddNodeTable creates the clean-room secure multi-node registry (v1.5).
var AddNodeTable = &gormigrate.Migration{
	ID: "20260712-add-node-table",
	Migrate: func(tx *gorm.DB) error {
		return tx.AutoMigrate(&model.Node{})
	},
}
