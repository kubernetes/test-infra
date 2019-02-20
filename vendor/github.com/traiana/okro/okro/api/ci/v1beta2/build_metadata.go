package v1beta2

type BuildMetadata struct {
	OkroURL             string `json:"okro_url"`
	Tenant              string `json:"tenant"`
	DockerBaseURL       string `json:"docker_base_url"`
	DockerConfigName    string `json:"docker_config_name"`
	BuildServiceAccount string `json:"build_service_account"`
	CIUtilImage         string `json:"ci_util_image"`
	CIUtilConfigName    string `json:"ci_util_config"`

	CIRepo            string `json:"ci_repo"`
	ImageNameTemplate string `json:"image_name_template"`

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
}
