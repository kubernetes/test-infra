package v1beta2

import (
	"strings"

	. "github.com/go-ozzo/ozzo-validation"

	"github.com/traiana/okro/okro/pkg/util/errorx"
	"github.com/traiana/okro/okro/pkg/util/validation"
)

var (
	resolverErr = errorx.Newf(
		`resolver must be %q, %q, or an explicit "tenantName/realmName"`,
		ResolverLocal, ResolverMain)

	taskResolverErr = errorx.New(
		"task resolver is only allowed using dependency management, " +
			"and only for API dependencies")

	resolverRule = By(validateResolverFunc(false))

	resolverRuleWithTask = By(validateResolverFunc(true))

	targetKindRule = valuesIn([]interface{}{TargetKindAPI, TargetKindTopic})

	targetErr = errorx.New(
		"target string must be a fully qualified resource name, " +
			"e.g.: tenants/bob/catalog/topics/messages-01")

	targetRule = By(func(value interface{}) error {
		s := value.(string)
		parts := strings.Split(s, "/")
		// basic format validation to prevent out-of-range errors later on.
		// the actual targets are validated against real references in the db.
		if len(parts) < 3 {
			return targetErr
		}
		return nil
	})
)

func (d Domain) Validate() error {
	if err := ValidateStruct(&d,
		Field(&d.Name, nameRule),
		Field(&d.Labels, labelsRule),
		Field(&d.Realms),
	); err != nil {
		return err
	}

	if len(d.Realms) > 0 {
		// unique realm name
		ddRealm := validation.Deduper{}
		for i, r := range d.Realms {
			if dup := ddRealm.Add(r.Name, "realms", i); dup != nil {
				return dup.AsNested("realm")
			}
		}

		// main realm exists
		if !ddRealm.Has(ResolverMain) {
			return errorx.New("a non-empty domain must contain a main realm")
		}
	}

	return nil
}

func (r Realm) Validate() error {
	if err := ValidateStruct(&r,
		Field(&r.Name, nameRule),
		Field(&r.Labels, labelsRule),
		Field(&r.Resolver, Required, resolverRule),
		Field(&r.Tasks),
		Field(&r.Resources),
	); err != nil {
		return err
	}

	// unique task name
	ddTask := validation.Deduper{}
	for i, t := range r.Tasks {
		if dup := ddTask.Add(t.Name, "tasks", i); dup != nil {
			return dup.AsNested("task")
		}
	}

	return nil
}

func (t Task) Validate() error {
	if err := ValidateStruct(&t,
		Field(&t.Name, nameRule),
		Field(&t.Labels, labelsRule),
		Field(&t.Resolver, resolverRule),
		Field(&t.Modules),
	); err != nil {
		return err
	}

	// unique module (build.module).
	// releases must be resolved into builds before this runs
	ddModule := validation.Deduper{}
	for i, m := range t.Modules {
		key := aggr(m.Build, m.Module)
		if dup := ddModule.Add(key, "modules", i); dup != nil {
			return dup.AsNested("module")
		}
	}

	return nil
}

func (m ModuleProjection) Validate() error {
	return ValidateStruct(&m,
		Field(&m.Build, Required, nameRefRule),
		// release will be resolved into build
		Field(&m.Module, Required, nameRefRule),
		Field(&m.Labels, labelsRule),
		Field(&m.Resolver, resolverRule),
		Field(&m.DependencyManagement),
		// everything else is set by the server
	)
}

func (dm DependencyManagement) Validate() error {
	return ValidateStruct(&dm,
		Field(&dm.Static),
		Field(&dm.Dynamic),
	)
}

func (s StaticDependencyManagement) Validate() error {
	if err := ValidateStruct(&s,
		Field(&s.Modify),
	); err != nil {
		return err
	}

	// unique modify target
	ddMod := validation.Deduper{}
	for i, m := range s.Modify {
		key := aggr(m.TargetKind, m.Target)
		if dup := ddMod.Add(key, "modify", i); dup != nil {
			return dup.AsNested("dependency modification")
		}
	}

	return nil
}

func (d DependencyRule) Validate() error {
	return ValidateStruct(&d,
		Field(&d.TargetKind, Required, targetKindRule),
		Field(&d.Target, Required, nameRefRule, targetRule),
		Field(&d.Resolver, getResolverRule(d.TargetKind)),
		Field(&d.Constraints, labelsRule),
	)
}

func (d DynamicDependencyManagement) Validate() error {
	if err := ValidateStruct(&d,
		Field(&d.Add),
		Field(&d.AddSelection),
	); err != nil {
		return err
	}

	// unique add
	ddAdd := validation.Deduper{}
	for i, a := range d.Add {
		key := aggr(a.TargetKind, a.Target)
		if dup := ddAdd.Add(key, "add", i); dup != nil {
			return dup.AsNested("dependency addition")
		}
	}

	// unique select
	ddSel := validation.Deduper{}
	for i, s := range d.AddSelection {
		key := s.GeneratedName()
		if dup := ddSel.Add(key, "select", i); dup != nil {
			return dup.AsNested("dependency selection (catalog+kind+match signature)")
		}
	}

	return nil
}

func (d DependencySelection) Validate() error {
	return ValidateStruct(&d,
		Field(&d.Catalog, Required, nameRefRule),
		Field(&d.TargetKind, Required, targetKindRule),
		Field(&d.Resolver, getResolverRule(d.TargetKind)),
		Field(&d.MatchLabels, Required, labelsRule),
		Field(&d.Constraints, labelsRule),
	)
}

func (r Resources) Validate() error {
	if err := ValidateStruct(&r,
		Field(&r.Topics),
	); err != nil {
		return err
	}

	// unique topic
	ddTopic := validation.Deduper{}
	for i, t := range r.Topics {
		if dup := ddTopic.Add(t.Name, "topics", i); dup != nil {
			return dup.AsNested("topic projection")
		}
	}

	return nil
}

func (tp TopicProjection) Validate() error {
	return ValidateStruct(&tp,
		Field(&tp.Name, nameRule),
		Field(&tp.Labels, labelsRule),
	)
}

func validateResolverFunc(allowTask bool) func(value interface{}) error {
	return func(value interface{}) error {
		s := value.(string)
		switch s {
		case ResolverTask:
			if allowTask {
				return nil
			} else {
				return taskResolverErr
			}
		case ResolverLocal, ResolverMain:
			return nil
		default:
			parts := strings.Split(s, "/")
			if len(parts) != 2 {
				return resolverErr
			}
			tenant, realm := parts[0], parts[1]
			if tenant == "" || realm == "" {
				return resolverErr
			}
			return nil
		}
	}
}

func getResolverRule(targetKind string) Rule {
	switch targetKind {
	case TargetKindAPI:
		return resolverRuleWithTask
	default:
		return resolverRule
	}
}
