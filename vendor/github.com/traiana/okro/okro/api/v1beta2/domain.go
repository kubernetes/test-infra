package v1beta2

import (
	"fmt"

	"github.com/traiana/okro/okro/pkg/util/hashx"
)

const (
	ResolverTask  = "task"
	ResolverLocal = "local"
	ResolverMain  = "main"
)

const (
	TargetKindAPI   = "api"
	TargetKindTopic = "topic"
)

type Domain struct {
	// domain name (same as target env)
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// realms in this domain
	Realms []*Realm `json:"realms,omitempty"`
}

type Realm struct {
	// realm name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// dependency location resolver (local, main, manual[tenant/realm]).
	// can be overridden in any child level.
	// required at realm level.
	Resolver string `json:"resolver"`

	// tasks
	Tasks []*Task `json:"tasks"`

	// todo policies
	// Policies []*Policy `json:"policies"`

	// resources
	Resources Resources `json:"resources"`
}

type Task struct {
	// task name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// dependency location resolver (local, main, manual[tenant/realm]).
	// can be overridden in any child level.
	Resolver string `json:"resolver"`

	// modules
	Modules []*ModuleProjection `json:"modules"`
}

type ModuleProjection struct {
	// build name (can specify release instead)
	Build string `json:"build"`

	// release name (can specify build instead)
	Release string `json:"release,omitempty"`

	// projected module name
	Module string `json:"module"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// dependency location resolver (local, main, manual[tenant/realm]).
	// can be overridden in any child level.
	Resolver string `json:"resolver"`

	// dependency management
	DependencyManagement *DependencyManagement `json:"dependency_management,omitempty"`

	// allocated ports (set by the server)
	Ports []*Port `json:"ports,omitempty"`

	// bindings (set by the server)
	Bindings []*Binding `json:"bindings,omitempty"`
}

func (m ModuleProjection) GeneratedName() string {
	return fmt.Sprintf("%s-%s", m.Build, m.Module)
}

// todo query undeclared bindings

type DependencyManagement struct {
	// static dependency management (declared at build)
	Static *StaticDependencyManagement `json:"static,omitempty"`

	// dynamic dependency management (undeclared dependencies)
	Dynamic *DynamicDependencyManagement `json:"dynamic,omitempty"`
}

type StaticDependencyManagement struct {
	// modify declared dependencies with custom rules
	Modify []*DependencyRule `json:"modify,omitempty"`
}

type DynamicDependencyManagement struct {
	// add dependencies only known at deploy time (specific)
	Add []*DependencyRule `json:"add,omitempty"`

	// add dependencies only known at deploy time (selection by labels)
	AddSelection []*DependencySelection `json:"add_selection,omitempty"`
}

type DependencyRule struct {
	// target kind
	TargetKind string `json:"target_kind"`

	// target
	Target string `json:"target"`

	// dependency location resolver (local, main, manual[tenant/realm]).
	// a dependency rule for an API dependency can also specify "task",
	// to resolve the API using a task-local module.
	Resolver string `json:"resolver"`

	// additional constraints to verify against the implementor
	Constraints map[string]string `json:"constraints,omitempty"`
}

func (d DependencyRule) GeneratedName(dep string) string {
	return hashx.Short(dep, d.TargetKind, d.Target)
}

type DependencySelection struct {
	// tenant catalog to select in (defaults to self).
	// when selecting in the self catalog - selects from all targets.
	// when selecting from a foreign catalog - selects from public targets.
	Catalog string `json:"catalog,omitempty"`

	// target kind
	TargetKind string `json:"target_kind"`

	// select catalog targets by labels.
	// all labels are required to make a target valid.
	MatchLabels map[string]string `json:"match_labels"`

	// dependency location resolver (local, main, manual[tenant/realm]).
	// a dependency selection for an API dependency can also specify "task",
	// to resolve the API using a task-local module.
	Resolver string `json:"resolver"`

	// additional constraints to verify against the implementor
	Constraints map[string]string `json:"constraints,omitempty"`
}

func (d DependencySelection) GeneratedName() string {
	s := []string{d.Catalog, d.TargetKind}
	for k, v := range d.MatchLabels {
		s = append(s, k, v)
	}
	return hashx.Short(s...)
}

type Port struct {
	// object metadata
	Meta `json:",inline"`

	// allocated for endpoint
	Endpoint string `json:"endpoint"`

	// port number (allocated by the server)
	Number int `json:"number"`
}

type Binding struct {
	// object metadata
	Meta `json:",inline"`

	// target kind
	TargetKind string `json:"target_kind"`

	// target composite key
	Target string `json:"target"`

	// dependency management indicator
	Dep string `json:"dep,omitempty"`

	// acting resolver
	Resolver string `json:"resolver"`

	// resolving composite key
	// e.g. tenant/realm/task/build|release/module
	ResolvedBy []string `json:"resolved_by"`
}

type Resources struct {
	// topic projections
	Topics []*TopicProjection `json:"topics,omitempty"`
}

type TopicProjection struct {
	// topic projection name (same as projection)
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`
}
