package repo

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
)

type NodeRepo struct{}

type INodeRepo interface {
	Get(opts ...global.DBOption) (model.Node, error)
	GetList(opts ...global.DBOption) ([]model.Node, error)
	Create(node *model.Node) error
	Update(id uint, vars map[string]interface{}) error
	Delete(opts ...global.DBOption) error

	// BurnEnrollment atomically consumes a pending enrollment for this node id
	// and nonce, flipping enrolled false->true in one statement. It returns
	// true only for the single caller that won the race (N1 replay defense).
	BurnEnrollment(id uint, nonce string) (bool, error)
}

func NewINodeRepo() INodeRepo {
	return &NodeRepo{}
}

func (r *NodeRepo) Get(opts ...global.DBOption) (model.Node, error) {
	var node model.Node
	db := global.DB
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.First(&node).Error
	return node, err
}

func (r *NodeRepo) GetList(opts ...global.DBOption) ([]model.Node, error) {
	var nodes []model.Node
	db := global.DB.Model(&model.Node{})
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.Find(&nodes).Error
	return nodes, err
}

func (r *NodeRepo) Create(node *model.Node) error {
	return global.DB.Create(node).Error
}

func (r *NodeRepo) Update(id uint, vars map[string]interface{}) error {
	return global.DB.Model(&model.Node{}).Where("id = ?", id).Updates(vars).Error
}

func (r *NodeRepo) Delete(opts ...global.DBOption) error {
	db := global.DB
	for _, opt := range opts {
		db = opt(db)
	}
	return db.Delete(&model.Node{}).Error
}

func (r *NodeRepo) BurnEnrollment(id uint, nonce string) (bool, error) {
	res := global.DB.Model(&model.Node{}).
		Where("id = ? AND enroll_nonce = ? AND enrolled = ?", id, nonce, false).
		Updates(map[string]interface{}{"enrolled": true, "status": "enrolling"})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
