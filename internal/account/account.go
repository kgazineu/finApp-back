package account

import (
	"time"

	"github.com/jmoiron/sqlx"
)

type Kind string

const (
	KindAsset     Kind = "asset"
	KindLiability Kind = "liability"
)

type Account struct {
	ID         int64
	Name       string
	Kind       Kind
	HasYield   bool
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

type Module struct {
	controller *Controller
	service    *Service
}

func NewModule(db *sqlx.DB) *Module {
	repo := NewRepository(db)
	service := NewService(repo)
	controller := NewController(service)
	return &Module{controller: controller, service: service}
}

func (m *Module) Service() *Service {
	return m.service
}
