package v1beta1

import (
	valid "github.com/go-ozzo/ozzo-validation"
)

func (m Meta) Validate() error {
	return valid.ValidateStruct(&m,
		valid.Field(&m.Name, valid.Required, nameRule),
		// other fields are set by the server
	)
}
