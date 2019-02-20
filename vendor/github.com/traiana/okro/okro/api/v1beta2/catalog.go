package v1beta2

import "time"

const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"
)

const (
	ProtocolHTTP = "http"
	ProtocolGRPC = "grpc"
	ProtocolTCP  = "tcp"
)

type Catalog struct {
	// object metadata
	Meta `json:",inline"`

	// tenant api groups
	APIGroups []*APIGroup `json:"api_groups"`

	// tenant topics
	Topics []*Topic `json:"topics"`
}

type Exposure struct {
	// visibility setting (private/public).
	// private: dependency declaration only available within the tenant.
	// public:  dependencies declaration available to all tenants.
	// this does not affect ingress settings.
	Visibility string `json:"visibility"`

	// end of life time.
	// when set, the component is considered deprecated.
	// when this time passes, the component is considered deprecated and "post-EOL".
	EOL *time.Time `json:"eol,omitempty"`
}

type APIGroup struct {
	// api group name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// preferred api (should be a version) for client usage
	PreferredAPI string `json:"preferred_api"`

	// apis
	APIs []*API `json:"apis"`
}

type API struct {
	// api name (e.g. v1alpha1, v1beta3, v1-http, v2-grpc).
	// all implementations of a given api are implicitly regarded as backwards compatible.
	// breaking changes should create a new api under the same group.
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline" db:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// api protocol (http/grpc/tcp)
	Protocol string `json:"protocol"`

	// api path.
	// http: usually /<api-group-name>/<api-name>, e.g. /audit/v1beta2.
	// grpc: the standard /pkg.Service, e.g. /io.okro.stuff.AuditService.
	// tcp: must be empty.
	// cannot be changed after creation.
	Path string `json:"path,omitempty"`

	// exposure setting for api-like objects
	Exposure `json:",inline"`
}

type Topic struct {
	// topic name
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// topic schema (top level message).
	// cannot be changed after creation.
	Schema string `json:"schema"`

	// topic partitions.
	// cannot be changed after creation.
	Partitions int `json:"partitions"`

	// exposure setting for api-like objects
	Exposure `json:",inline"`
}
