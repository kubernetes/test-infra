package v1beta2

import (
	"strings"

	. "github.com/go-ozzo/ozzo-validation"

	"github.com/traiana/okro/okro/pkg/util/errorx"
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
		ddRealm := deduper{}
		for i, r := range d.Realms {
			if dup := ddRealm.add(r.Name, "realms", i); dup != nil {
				return dup.asNested("realm")
			}
		}

		// main realm exists
		if !ddRealm.has(ResolverMain) {
			return errorx.New("a non-empty domain must contain a main realm")
		}
	}

	return nil
}

func (r Realm) Validate() error {
	if err := ValidateStruct(&r,
		Field(&r.Name, nameRule),
		Field(&r.Labels, labelsRule),
		Field(&r.Resolver, resolverRule),
		Field(&r.Tasks),
		Field(&r.Resources),
	); err != nil {
		return err
	}

	// unique task name
	ddTask := deduper{}
	for i, t := range r.Tasks {
		if dup := ddTask.add(t.Name, "tasks", i); dup != nil {
			return dup.asNested("task")
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
	ddModule := deduper{}
	for i, m := range t.Modules {
		key := aggr(m.Build, m.Module)
		if dup := ddModule.add(key, "modules", i); dup != nil {
			return dup.asNested("module")
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
	ddMod := deduper{}
	for i, m := range s.Modify {
		key := aggr(m.TargetKind, m.Target)
		if dup := ddMod.add(key, "modify", i); dup != nil {
			return dup.asNested("dependency modification")
		}
	}

	return nil
}

func (dr DependencyRule) Validate() error {
	return ValidateStruct(&dr,
		Field(&dr.TargetKind, Required, targetKindRule),
		Field(&dr.Target, Required, nameRefRule, targetRule),
		Field(&dr.Resolver, getResolverRule(dr.TargetKind)),
		Field(&dr.Constraints, labelsRule),
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
	ddAdd := deduper{}
	for i, a := range d.Add {
		key := aggr(a.TargetKind, a.Target)
		if dup := ddAdd.add(key, "add", i); dup != nil {
			return dup.asNested("dependency addition")
		}
	}

	// unique select
	ddSel := deduper{}
	for i, s := range d.AddSelection {
		key := s.Sig()
		if dup := ddSel.add(key, "select", i); dup != nil {
			return dup.asNested("dependency selection (catalog+kind+match signature)")
		}
	}

	return nil
}

func (ds DependencySelection) Validate() error {
	return ValidateStruct(&ds,
		Field(&ds.Catalog, Required, nameRefRule),
		Field(&ds.TargetKind, Required, targetKindRule),
		Field(&ds.Resolver, getResolverRule(ds.TargetKind)),
		Field(&ds.MatchLabels, Required, labelsRule),
		Field(&ds.Constraints, labelsRule),
	)
}

func (r Resources) Validate() error {
	if err := ValidateStruct(&r,
		Field(&r.Topics),
	); err != nil {
		return err
	}

	// unique topic
	ddTopic := deduper{}
	for i, t := range r.Topics {
		if dup := ddTopic.add(t.Name, "topics", i); dup != nil {
			return dup.asNested("topic projection")
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
