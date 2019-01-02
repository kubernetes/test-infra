package v1beta1

import (
	valid "github.com/go-ozzo/ozzo-validation"
)

func (t Tenant) Validate() error {
	if err := valid.ValidateStruct(&t,
		valid.Field(&t.Meta),
		valid.Field(&t.DisplayName, valid.Required),
		valid.Field(&t.CatalogSourceURL, valid.Required, gitHubRepo),
		valid.Field(&t.DomainSources),
	); err != nil {
		return err
	}

	ddDS := deduper{}
	for i, ds := range t.DomainSources {
		if dup := ddDS.add(ds.Name, "domains", i); dup != nil {
			return dup.asNested("domain")
		}
	}

	return nil
}

func (ds TenantDomainSource) Validate() error {
	return valid.ValidateStruct(&ds,
		valid.Field(&ds.Name, valid.Required, nameRule),
		valid.Field(&ds.URL, valid.Required, gitHubRepo),
	)
}
