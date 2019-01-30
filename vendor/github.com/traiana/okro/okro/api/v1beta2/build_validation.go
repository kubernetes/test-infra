package v1beta2

import (
	. "github.com/go-ozzo/ozzo-validation"

	"github.com/traiana/okro/okro/pkg/util/errorx"
)

func (b Build) Validate() error {
	if err := ValidateStruct(&b,
		Field(&b.Name, nameRule),
		Field(&b.Labels, labelsRule),
		Field(&b.Pipeline, Required),
		Field(&b.PipelineRevision, Required),
		Field(&b.Source, Required),
		Field(&b.SourceRef, Required),
		Field(&b.SourceRevision, Required),
		Field(&b.Artifacts),
		Field(&b.Modules),
	); err != nil {
		return err
	}

	// todo validate build name matches hashing

	if b.Dead {
		return errorx.Errors{
			"dead": errorx.New("can't create dead build"),
		}
	}

	// unique artifact name
	ddArtifact := deduper{}
	for i, a := range b.Artifacts {
		if dup := ddArtifact.add(a.Name, "artifacts", i); dup != nil {
			return dup.asNested("artifact")
		}
	}

	// unique module name, valid module artifact
	ddModule := deduper{}
	for i, m := range b.Modules {
		if dup := ddModule.add(m.Name, "modules", i); dup != nil {
			return dup.asNested("module")
		}
		if !ddArtifact.has(m.Artifact) {
			err := errorx.Newf("unknown artifact reference %q", m.Artifact)
			return errorx.Deep(err, "modules", i)
		}
	}

	return nil
}

func (a Artifact) Validate() error {
	return ValidateStruct(&a,
		Field(&a.Name, nameRule),
		Field(&a.Labels, labelsRule),
		Field(&a.Type, Required), // todo validate known types
		Field(&a.URL, Required),  // todo validate url
	)
}

func (m Module) Validate() error {
	if err := ValidateStruct(&m,
		Field(&m.Name, nameRule),
		Field(&m.Labels, labelsRule),
		Field(&m.Description, Length(0, 280)),
		Field(&m.Artifact, Required, nameRefRule),
		Field(&m.Endpoints),
		Field(&m.Wants),
	); err != nil {
		return err
	}

	// unique endpoint name
	ddEndpoint := deduper{}
	for i, ep := range m.Endpoints {
		if dup := ddEndpoint.add(ep.Name, "endpoints", i); dup != nil {
			return dup.asNested("endpoint")
		}
	}

	return nil
}

func (ep Endpoint) Validate() error {
	if err := ValidateStruct(&ep,
		Field(&ep.Name, nameRule),
		Field(&ep.Labels, labelsRule),
		Field(&ep.Protocol, Required, protocolRule),
		Field(&ep.Serves),
	); err != nil {
		return err
	}

	// unique serve
	ddSrv := deduper{}
	for i, s := range ep.Serves {
		key := aggr(s.Tenant, s.APIGroup, s.API)
		if dup := ddSrv.add(key, "serves", i); dup != nil {
			return dup.asNested("serve")
		}
	}

	return nil
}

func (w Wants) Validate() error {
	if err := ValidateStruct(&w,
		Field(&w.APIs),
		Field(&w.Topics),
	); err != nil {
		return err
	}

	// unique apis
	ddAPIs := deduper{}
	for i, a := range w.APIs {
		key := aggr(a.Tenant, a.APIGroup, a.API)
		if dup := ddAPIs.add(key, "apis", i); dup != nil {
			return dup.asNested("api dependency")
		}
	}

	// unique topics
	ddTopics := deduper{}
	for i, a := range w.Topics {
		key := aggr(a.Tenant, a.Topic)
		if dup := ddTopics.add(key, "topics", i); dup != nil {
			return dup.asNested("topic dependency")
		}
	}

	return nil
}

func (k APIKey) Validate() error {
	return ValidateStruct(&k,
		Field(&k.Tenant, Required, nameRefRule),
		Field(&k.APIGroup, Required, nameRefRule),
		Field(&k.API, Required, nameRefRule),
	)
}

func (t TopicKey) Validate() error {
	return ValidateStruct(&t,
		Field(&t.Tenant, Required, nameRefRule),
		Field(&t.Topic, Required, nameRefRule),
	)
}
