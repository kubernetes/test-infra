/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"encoding/json"
)

// Key types specify the way Spyglass will fetch artifact handles
const (
	GCSKeyType  = "gcs"
	ProwKeyType = "prowjob"
)

// Lens defines the interface that lenses are required to implement in order to be used by Spyglass.
type Lens interface {
	// Header returns a a string that is injected into the rendered lens's <head>
	Header(artifacts []Artifact, resourceRoot string, config json.RawMessage) string
	// Body returns a string that is initially injected into the rendered lens's <body>.
	// The lens's front-end code may call back to Body again, passing in some data string of its choosing.
	Body(artifacts []Artifact, resourceRoot string, data string, config json.RawMessage) string
	// Callback receives a string sent by the lens's front-end code and returns another string to be returned
	// to that frontend code.
	Callback(artifacts []Artifact, resourceRoot string, data string, config json.RawMessage) string
}

// Artifact represents some output of a prow job
type Artifact interface {
	// ReadAt reads len(p) bytes of the artifact at offset off. (unsupported on some compressed files)
	ReadAt(p []byte, off int64) (n int, err error)
	// ReadAtMost reads at most n bytes from the beginning of the artifact
	ReadAtMost(n int64) ([]byte, error)
	// CanonicalLink gets a link to viewing this artifact in storage
	CanonicalLink() string
	// JobPath is the path to the artifact within the job (i.e. without the job prefix)
	JobPath() string
	// ReadAll reads all bytes from the artifact up to a limit specified by the artifact
	ReadAll() ([]byte, error)
	// ReadTail reads the last n bytes from the artifact (unsupported on some compressed files)
	ReadTail(n int64) ([]byte, error)
	// Size gets the size of the artifact in bytes, may make a network call
	Size() (int64, error)
}

// RequestAction defines the action for a request
type RequestAction string

const (
	// RequestActionInitial means that this is the initial request for the given lense
	RequestActionInitial RequestAction = "initial"
	// RequestActionRerender means that this is a request to re-render the lenses body
	RequestActionRerender RequestAction = "rerender"
	// ResponseCallback means that this is an arbitrary callback
	RequestActionCallBack RequestAction = "callback"
)

type LensRequest struct {
	// Action is the specific type of request being made
	Action RequestAction `json:"action"`
	// Data is a string of data passed back from the lens frontend
	Data string `json:"data,omitepty"`
	// Config is the config for the lens, if any, in a lens-defined format
	Config json.RawMessage `json:"config,omitepty"`
	// ResourceRoot is a URL at which the lens's own resources can be accessed
	// by the client browser.
	ResourceRoot string `json:"resourceRoot"`
	// Artifacts contains the artifacts for this request
	Artifacts []string `json:"artifacts"`
	// ArtifactSource is the source from which to fetch the artifacts
	ArtifactSource string
	// LensIndex is the index by which the lens config can be found
	// TODO: Replace with something proper or avoid needing this
	LensIndex int `json:"index"`
}
