package v1beta1

type Release struct {
	Meta  `json:",inline"`
	Build *Build `json:"build"`
}

type Build struct {
	Meta      `json:",inline"`
	Revision  string      `json:"revision"`
	Dead      bool        `json:"dead,omitempty"`
	Artifacts []*Artifact `json:"artifacts"`
	Modules   []*Module   `json:"modules,omitempty"`
}

type Artifact struct {
	Meta `json:",inline"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Module struct {
	Meta     `json:",inline"`
	Artifact string     `json:"artifact"`
	Dead     bool       `json:"dead,omitempty"`
	Ports    []*Port    `json:"ports,omitempty"`
	Serves   []*Serving `json:"serves,omitempty"`
	Wants    *Wants     `json:"wants,omitempty"`
}

type Port struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`           // http/grpc/tcp
	Internal bool   `json:"internal,omitempty"` // used only by the platform (e.g. metrics, health)
}

type Wants struct {
	APIs   []*APIVersionKey `json:"apis,omitempty"`
	Topics []*TopicKey      `json:"topics,omitempty"`
}

type APIVersionKey struct {
	Tenant  string `json:"tenant,omitempty"`
	App     string `json:"app"`
	API     string `json:"api"`
	Version string `json:"version"`
}

type Serving struct {
	Port    string `json:"port"`
	API     string `json:"api"`
	Version string `json:"version"`
}

type TopicKey struct {
	Tenant string `json:"tenant,omitempty"`
	Pubsub string `json:"pubsub"`
	Topic  string `json:"topic"`
}
