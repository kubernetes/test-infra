package v1beta1

import (
	"fmt"

	valid "github.com/go-ozzo/ozzo-validation"
	"github.com/traiana/okro/okro/pkg/util/errorx"
)

func (b Build) Validate() error {
	if err := valid.ValidateStruct(&b,
		valid.Field(&b.Meta),
		valid.Field(&b.Revision, valid.Required),
		valid.Field(&b.Artifacts, valid.Required),
		valid.Field(&b.Modules),
	); err != nil {
		return err
	}

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
			derr := fmt.Errorf("unknown artifact reference %q", m.Artifact)
			return errorx.Deep(derr, "modules", i)
		}
	}

	return nil
}

func (r Release) Validate() error {
	return valid.ValidateStruct(&r,
		valid.Field(&r.Meta),
		valid.Field(&r.Build, valid.Required),
	)
}

func (a Artifact) Validate() error {
	return valid.ValidateStruct(&a,
		valid.Field(&a.Meta),
		valid.Field(&a.Type, valid.Required), // todo validate known types
		valid.Field(&a.URL, valid.Required),
	)
}

func (m Module) Validate() error {
	if err := valid.ValidateStruct(&m,
		valid.Field(&m.Meta),
		valid.Field(&m.Artifact, valid.Required),
		valid.Field(&m.Ports),
		valid.Field(&m.Serves),
		valid.Field(&m.Wants),
	); err != nil {
		return err
	}

	// unique port names
	ddPorts := deduper{}
	portsByName := map[string]*Port{}
	unboundPorts := map[string]bool{}
	for i, p := range m.Ports {
		if dup := ddPorts.add(p.Name, "ports", i); dup != nil {
			return dup.asNested("port")
		}
		portsByName[p.Name] = p
		if !p.Internal {
			unboundPorts[p.Name] = true
		}
	}

	// unique servings, port references
	ddSrv := deduper{}
	for i, s := range m.Serves {
		p, ok := portsByName[s.Port]
		if !ok {
			derr := fmt.Errorf("unknown port reference %q", s.Port)
			return errorx.Deep(derr, "serves", i)
		}
		if p.Internal {
			derr := fmt.Errorf("cannot serve on internal port %q", s.Port)
			return errorx.Deep(derr, "serves", i)
		}
		key := aggr(s.API, s.Version, s.Port)
		if dup := ddSrv.add(key, "serves", i); dup != nil {
			return dup.asNested("serving")
		}
		delete(unboundPorts, s.Port)
	}

	// check for unbound ports
	if len(unboundPorts) > 0 {
		errs := errorx.Errors{}
		for port := range unboundPorts {
			errs.InsertVar(fmt.Errorf("unbound external port"), port)
		}
		return errorx.Errors{"ports": errs}
	}

	return nil
}

func (p Port) Validate() error {
	return valid.ValidateStruct(&p,
		valid.Field(&p.Name, valid.Required, nameRule),
		valid.Field(&p.Protocol, valid.Required, protocolRule),
	)
}

func (s Serving) Validate() error {
	return valid.ValidateStruct(&s,
		valid.Field(&s.Port, valid.Required),
		valid.Field(&s.API, valid.Required),
		valid.Field(&s.Version, valid.Required),
	)
}

func (w Wants) Validate() error {
	if err := valid.ValidateStruct(&w,
		valid.Field(&w.APIs),
		valid.Field(&w.Topics),
	); err != nil {
		return err
	}

	// unique apis
	ddAPIs := deduper{}
	for i, a := range w.APIs {
		key := aggr(a.Tenant, a.App, a.API, a.Version)
		if dup := ddAPIs.add(key, "apis", i); dup != nil {
			return dup.asNested("api dependency")
		}
	}

	// unique topics
	ddTopics := deduper{}
	for i, a := range w.Topics {
		key := aggr(a.Tenant, a.Pubsub, a.Topic)
		if dup := ddTopics.add(key, "topics", i); dup != nil {
			return dup.asNested("topic dependency")
		}
	}

	return nil
}

func (k APIVersionKey) Validate() error {
	return valid.ValidateStruct(&k,
		valid.Field(&k.Tenant, valid.Required),
		valid.Field(&k.App, valid.Required),
		valid.Field(&k.API, valid.Required),
		valid.Field(&k.Version, valid.Required),
	)
}

func (t TopicKey) Validate() error {
	return valid.ValidateStruct(&t,
		valid.Field(&t.Tenant, valid.Required),
		valid.Field(&t.Pubsub, valid.Required),
		valid.Field(&t.Topic, valid.Required),
	)
}
