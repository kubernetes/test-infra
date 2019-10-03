/*
Copyright 2019 The Kubernetes Authors.

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

package main

// These types are copied from https://github.com/kubernetes-sigs/k8s-container-image-promoter/blob/50e6f45fa9499c9b67c366af3fc33ce328a026dc/lib/dockerregistry/types.go

// Manifest stores the information in a manifest file (describing the
// desired state of a Docker Registry).
type Manifest struct {
	// Registries contains the source and destination (Src/Dest) registry names.
	// It is possible that in the future, we support promoting to multiple
	// registries, in which case we would have more than just Src/Dest.
	Registries []RegistryContext `yaml:"registries,omitempty"`
	Images     []Image           `yaml:"images,omitempty"`

	// A rename list can contain a list of paths, where each path is a string.
	//
	// - A rename entry must have at least 2 paths, one for the source, another
	// for at least 1 dest registry.
	//
	// - Any unknown registry entries in here will be considered a parsing
	// error.
	//
	// - Any redundant entries in here will be considered a parsing error. E.g.,
	// "gcr.io/louhi-qa/glbc:gcr.io/louhi-gke-k8s/glbc" is redunant as it is
	// implied already.
	//
	// - The names must be valid paths (no errant punctuation, etc.).
	//
	// - No self-loops allowed (a registry must not appear more than 1 time).
	//
	// - Each name must be the registry+pathname, *without* a trailing slash.
	//
	// Just before the promotion, each rename entry is processed, to update the
	// master inventory entries for the *renamed* images.
	//
	// When fetching data from a renamed image's repository, they are
	// "normalized" to the path as seen in the source registry for that image
	// --- this is so that the set difference logic can be used as-is. Only when
	// the promotion itself is performed, do we "denormalize" at the very last
	// moment by modifying the argument to each destination path.
	Renames []Rename `yaml:"renames,omitempty"`
}

// Image holds information about an image. It's like an "Object" in the OOP
// sense, and holds all the information relating to a particular image that we
// care about.
type Image struct {
	ImageName string     `yaml:"name"`
	Dmap      DigestTags `yaml:"dmap,omitempty"`
}

// Rename is list of paths, where each path is full
// image name (registry + image name, without the tag).
type Rename []RegistryImagePath

// RegistryImagePath is the registry name and image name, without the tag. E.g.
// "gcr.io/foo/bar/baz/image".
type RegistryImagePath string

// DigestTags is a map where each digest is associated with a TagSlice. It is
// associated with a TagSlice because an image digest can have more than 1 tag
// pointing to it, even within the same image name's namespace (tags are
// namespaced by the image name).
type DigestTags map[string][]string

// RegistryContext holds information about a registry, to be written in a
// manifest file.
type RegistryContext struct {
	Name           string `yaml:"name,omitempty"`
	ServiceAccount string `yaml:"service-account,omitempty"`
	Src            bool   `yaml:"src,omitempty"`
}
