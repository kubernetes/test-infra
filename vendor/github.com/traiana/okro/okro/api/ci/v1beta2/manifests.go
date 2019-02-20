package v1beta2

import (
	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

type OkroManifest struct {
	Modules []*okrov1beta2.Module `json:"modules,omitempty"`
}
