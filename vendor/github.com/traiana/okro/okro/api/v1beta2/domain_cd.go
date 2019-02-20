package v1beta2

// todo should add catalog/build/domain labels to CRD?

type PortCd struct {
	// port name, i.e endpoint name
	Name string `json:"endpoint"`

	// number
	Number int `json:"number"`

	// protocol
	Protocol string `json:"protocol"`
}

type ModuleCd struct {
	// projected module name
	Module string `json:"module"`

	// artifact url
	Image string `json:"image"`

	// ports
	Ports []*PortCd `json:"ports"`
}

type TaskCd struct {
	// task name
	Name string `json:"name"`

	// modules
	Modules []*ModuleCd `json:"modules"`
}

type TopicCd struct {
	// task name
	Name string `json:"name"`

	// topic schema (top level message).
	// cannot be changed after creation.
	Schema string `json:"schema"`

	// topic partitions.
	// cannot be changed after creation.
	Partitions int `json:"partitions"`
}

type RealmCd struct {
	// realm name
	Name string `json:"name"`

	// tasks
	Tasks []*TaskCd `json:"tasks"`

	// todo policies
	// Policies []*Policy `json:"policies"`

	// resources
	Topics []*TopicCd `json:"topics,omitempty"`
}

type DomainCd struct {
	// domain name (same as target env)
	Name string `json:"name"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// realms in this domain
	Realms []*RealmCd `json:"realms,omitempty"`
}

type EnvCd struct {
	// environment name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// domains in this env (read only)
	Domains []*DomainCd `json:"domains"`
}
