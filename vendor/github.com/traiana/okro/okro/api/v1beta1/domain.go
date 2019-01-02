package v1beta1

const (
	LookupLocal = "local"
	LookupMain  = "main"
)

type Domain struct {
	Meta          `json:",inline"`
	DisplayName   string   `json:"display_name"`
	TenantDomains []string `json:"tenant_domains"`
}

type TenantDomain struct {
	Meta      `json:",inline"`
	Lookup    string   `json:"lookup"`
	MainRealm string   `json:"main_realm"`
	Realms    []*Realm `json:"realms"`
}

type Realm struct {
	Meta    `json:",inline"`
	Lookup  string              `json:"lookup,omitempty"`
	Apps    []*AppProjection    `json:"apps"`
	Pubsubs []*PubsubProjection `json:"pubsubs"`
}

type AppProjection struct {
	Meta    `json:",inline"`
	Lookup  string              `json:"lookup,omitempty"`
	Build   string              `json:"build"`
	Release string              `json:"release,omitempty"`
	Modules []*ModuleProjection `json:"modules"`
}

type ModuleProjection struct {
	Meta   `json:",inline"`
	Binds  *Binds `json:"binds"`
	Lookup string `json:"lookup,omitempty"`
	// todo resources, monitoring, scale, etc..
}

type Binds struct {
	APIs   []*APIBinding   `json:"apis,omitempty"`
	Topics []*TopicBinding `json:"topics,omitempty"`
}

type APIBinding struct {
	Lookup  string `json:"lookup,omitempty"`
	Tenant  string `json:"tenant,omitempty"`
	App     string `json:"app"`
	API     string `json:"api"`
	Version string `json:"version"`
	At      struct {
		Realm  string `json:"realm"`
		Module string `json:"module"`
	} `json:"at"`
}

type TopicBinding struct {
	Lookup string `json:"lookup,omitempty"`
	Tenant string `json:"tenant,omitempty"`
	Pubsub string `json:"pubsub"`
	Topic  string `json:"topic"`
	At     struct {
		Realm string `json:"realm"`
	} `json:"at"`
}

type PubsubProjection struct {
	Meta   `json:",inline"`
	Topics []*TopicProjection `json:"topics"`
}

type TopicProjection struct {
	Meta `json:",inline"`
}
