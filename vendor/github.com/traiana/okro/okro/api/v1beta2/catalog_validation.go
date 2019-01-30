package v1beta2

import (
	. "github.com/go-ozzo/ozzo-validation"

	"github.com/traiana/okro/okro/pkg/util/errorx"
)

var (
	visibilities     = []interface{}{VisibilityPrivate, VisibilityPublic}
	visibilitiesRule = valuesIn(visibilities)
)

func (c Catalog) Validate() error {
	if err := ValidateStruct(&c,
		Field(&c.APIGroups),
		Field(&c.Topics),
	); err != nil {
		return err
	}

	// unique api group name
	ddAPIGroup := deduper{}
	for i, a := range c.APIGroups {
		if dup := ddAPIGroup.add(a.Name, "apigroups", i); dup != nil {
			return dup.asNested("api group")
		}
	}

	// unique topic name
	ddTopic := deduper{}
	for i, p := range c.Topics {
		if dup := ddTopic.add(p.Name, "topics", i); dup != nil {
			return dup.asNested("topic")
		}
	}

	return nil
}

func (ag APIGroup) Validate() error {
	if err := ValidateStruct(&ag,
		Field(&ag.Name, nameRule),
		Field(&ag.Labels, labelsRule),
		Field(&ag.PreferredAPI, Required),
		Field(&ag.APIs, Required),
	); err != nil {
		return err
	}

	// unique api name per group
	ddAPI := deduper{}
	for i, v := range ag.APIs {
		if dup := ddAPI.add(v.Name, "apis", i); dup != nil {
			return dup.asNested("api")
		}
	}

	// preferred api declared
	if !ddAPI.has(ag.PreferredAPI) {
		err := errorx.Newf("unknown api %q", ag.PreferredAPI)
		return errorx.Errors{"preferred_api": err}
	}

	// unique path per api group (only http/grpc)
	ddPath := deduper{}
	for i, a := range ag.APIs {
		switch a.Protocol {
		case ProtocolHTTP, ProtocolGRPC:
			if dup := ddPath.add(a.Path, "apis", i); dup != nil {
				return dup.asNested("api path")
			}
		}
	}

	return nil
}

func (a API) Validate() error {
	// todo check path is valid url
	if err := ValidateStruct(&a,
		Field(&a.Name, nameRule),
		Field(&a.Labels, labelsRule),
		Field(&a.Protocol, Required, protocolRule),
		Field(&a.Exposure),
	); err != nil {
		return err
	}

	if a.Protocol == ProtocolTCP && a.Path != "" {
		err := errorx.New("tcp path must be empty")
		return errorx.Errors{"path": err}
	}

	return nil
}

func (t Topic) Validate() error {
	// todo check schema is valid pkg/msg
	return ValidateStruct(&t,
		Field(&t.Name, nameRule),
		Field(&t.Labels, labelsRule),
		Field(&t.Schema, Required),
		Field(&t.Partitions, Required, Min(1)),
		Field(&t.Exposure),
	)
}

func (e Exposure) Validate() error {
	return ValidateStruct(&e,
		Field(&e.Visibility, Required, visibilitiesRule),
	)
}
