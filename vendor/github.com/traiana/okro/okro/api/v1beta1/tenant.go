package v1beta1

type Tenant struct {
	Meta             `json:",inline"`
	DisplayName      string               `json:"display_name"`
	CatalogSourceURL string               `json:"catalog_url"`
	DomainSources    []TenantDomainSource `json:"domains"`
}

type TenantDomainSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
