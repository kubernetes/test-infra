package v1beta2

import (
	. "github.com/go-ozzo/ozzo-validation"
)

func (e Env) Validate() error {
	return ValidateStruct(&e,
		Field(&e.Name, nameRule),
		Field(&e.DisplayName, displayNameRule),
	)
}

func (t Tenant) Validate() error {
	if err := ValidateStruct(&t,
		Field(&t.Name, nameRule),
		Field(&t.DisplayName, displayNameRule),
		Field(&t.CatalogURL, Required, gitURLRule),
		Field(&t.CIURL, Required, gitURLRule),
		Field(&t.DomainURLs),
	); err != nil {
		return err
	}

	ddDomainURLs := deduper{}
	for i, ds := range t.DomainURLs {
		if dup := ddDomainURLs.add(ds.Env, "domains", i); dup != nil {
			return dup.asNested("domain")
		}
	}

	return nil
}

func (d DomainURL) Validate() error {
	return ValidateStruct(&d,
		Field(&d.Env, Required, nameRefRule),
		Field(&d.URL, Required, gitURLRule),
	)
}
