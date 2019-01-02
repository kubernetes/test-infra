package v1beta1

import (
	"time"
)

const (
	VISIBILITY_PRIVATE   = "private"
	VISIBILITY_WHITELIST = "whitelist"
	VISIBILITY_PUBLIC    = "public"
)

const (
	PROTOCOL_HTTP = "http"
	PROTOCOL_GRPC = "grpc"
	PROTOCOL_TCP  = "tcp"
)

type Catalog struct {
	Meta    `json:",inline"`
	Apps    []*App    `json:"apps,omitempty"`
	Pubsubs []*Pubsub `json:"pubsubs,omitempty"`
}

type App struct {
	Meta    `json:",inline"`
	RepoURL string `json:"repo_url"`
	Dead    bool   `json:"dead,omitempty"`
	APIs    []*API `json:"apis,omitempty"`
}

type API struct {
	Meta             `json:",inline"`
	Protocol         string        `json:"protocol"`
	PreferredVersion string        `json:"preferred_version"`
	Versions         []*APIVersion `json:"versions,omitempty"`
}

type APIVersion struct {
	Meta     `json:",inline"`
	Path     string `json:"path,omitempty"`
	Exposure `json:",inline"`
}

type Pubsub struct {
	Meta   `json:",inline"`
	Topics []*Topic `json:"topics,omitempty"`
}

type Topic struct {
	Meta       `json:",inline"`
	Schema     string `json:"schema"`
	Partitions int    `json:"partitions"`
	Exposure   `json:",inline"`
}

type Exposure struct {
	VisibilityLevel     string     `json:"visibility_level"`
	VisibilityWhitelist []string   `json:"visibility_whitelist,omitempty"`
	DeprecationEOL      *time.Time `json:"deprecation_eol,omitempty"`
}
