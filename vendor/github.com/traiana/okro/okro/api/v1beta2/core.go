package v1beta2

import (
	"time"
)

type Meta struct {
	// created timestamp (populated by server)
	CreatedAt *time.Time `json:"created_at"`

	// update timestamp (populated by server)
	UpdatedAt *time.Time `json:"updated_at"`

	// updated by user/bot id.
	// in case of gitops-based bot updates, the format is "<bot>:<repo>:<hash>".
	// updated for a change to this entity or a child entity.
	UpdatedBy string `json:"updated_by"`
}

type Env struct {
	// environment name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// user-facing name
	DisplayName string `json:"display_name"`

	// sensitive flag (prod-like)
	Sensitive bool `json:"sensitive,omitempty"`

	// tenants hosting domains in this env (read only)
	Tenants []string `json:"tenants"`
}

type Tenant struct {
	// tenant name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// user-facing name
	DisplayName string `json:"display_name"`

	// catalog git repo url (ssh)
	CatalogURL string `json:"catalog_url"`

	// ci git repo url (ssh)
	CIURL string `json:"ci_url"`

	// domain git repo urls (ssh)
	DomainURLs []*DomainURL `json:"domain_urls"`
}

type DomainURL struct {
	// target env
	Env string `json:"env"`

	// git repo url (ssh)
	URL string `json:"url"`
}
