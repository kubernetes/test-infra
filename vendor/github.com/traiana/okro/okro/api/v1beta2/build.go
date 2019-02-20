package v1beta2

type Build struct {
	// build name.
	// format: <short-source>-<sourceRevision>-<shorthash(pipeline,pipelineRevision)>
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// ci pipeline name
	Pipeline string `json:"pipeline"`

	// ci pipeline commit hash
	PipelineRevision string `json:"pipeline_revision"`

	// source repo url
	SourceURL string `json:"source_url"`

	// source repo owner
	SourceOwner string `json:"source_owner"`

	// source repo name
	SourceName string `json:"source_name"`

	// source ref (branch)
	SourceRef string `json:"source_ref"`

	// source repo commit hash
	SourceRevision string `json:"source_revision"`

	// marked dead by user / cleanup
	Dead bool `json:"dead,omitempty"`

	// build promotion (optional)
	Release *Release `json:"release,omitempty"`

	// build artifact
	Artifacts []*Artifact `json:"artifacts"`

	// build modules
	Modules []*Module `json:"modules"`
}

type Release struct {
	// release name (semver)
	Name string `json:"name"`

	// object metadata
	Meta `json:",inline"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`
}

type Artifact struct {
	// artifact name
	Name string `json:"name"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// artifact type (file, lib, image)
	Type string `json:"type"`

	// artifact storage location (s3/artifactory url, depending on type)
	URL string `json:"url"`
}

type Module struct {
	// module name
	Name string `json:"name"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// user-friendly description of this module's main functions
	Description string `json:"description,omitempty"`

	// artifact this module runs
	Artifact string `json:"artifact"`

	// logical endpoints exposed by this module
	Endpoints []*Endpoint `json:"endpoints"`

	// static requirements
	Wants Wants `json:"wants,omitempty"`

	// wants the ability to dynamically bind resources
	WantsDynamic bool `json:"wants_dynamic,omitempty"`
}

type Endpoint struct {
	// endpoint name
	Name string `json:"name"`

	// object labels
	Labels map[string]string `json:"labels,omitempty"`

	// protocol (http/grpc/tcp).
	// in reality ports can serve multiple protocols,
	// but it's easier to secure and reason about single function ports.
	Protocol string `json:"protocol"`

	// mark as a platform-only endpoint.
	// internal endpoints cannot be externally exposed,
	// and are not required to declare apis.
	Internal bool `json:"internal"`

	// apis served on this endpoint
	Serves []*APIKey `json:"serves,omitempty"`
}

type Wants struct {
	// apis statically required by this module
	APIs []*APIKey `json:"apis,omitempty"`

	// topics statically required by this module
	Topics []*TopicKey `json:"topics,omitempty"`
}

type APIKey struct {
	// owner tenant (optional - defaults to same tenant)
	Tenant string `json:"tenant"`

	// api group name
	APIGroup string `json:"api_group"`

	// api name
	API string `json:"api"`
}

type TopicKey struct {
	// owner tenant (optional - defaults to same tenant)
	Tenant string `json:"tenant"`

	// topic name
	Topic string `json:"topic"`
}
