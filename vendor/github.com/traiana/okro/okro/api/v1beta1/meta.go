package v1beta1

import (
	"time"
)

type Meta struct {
	Name      string     `json:"name"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
}
