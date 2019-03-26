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

package encoding

import (
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/test-infra/experiment/bazinga/pkg/config"
)

// Scheme is the runtime.Scheme to which all `bazinga` config API versions and types are registered.
var Scheme = runtime.NewScheme()

// Codecs provides access to encoding and decoding for the scheme.
var Codecs = serializer.NewCodecFactory(Scheme)

func init() {
	AddToScheme(Scheme)
}

// AddToScheme builds the scheme using all known `bazinga` API versions.
func AddToScheme(scheme *runtime.Scheme) {
	utilruntime.Must(config.AddToScheme(scheme))
	utilruntime.Must(scheme.SetVersionPriority(config.SchemeGroupVersion))
}

// Load reads the file at path and attempts to convert into a `bazinga` Config.
// The file may be one of the different API versions defined in scheme.
// A default config is returned when given an empty path.
func Load(path string) (*config.App, error) {
	if path == "" {
		return &config.App{}, nil
	}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	appConfig := &config.App{}
	if _, _, err := Codecs.UniversalDecoder().Decode(
		buf, nil, appConfig); err != nil {
		return nil, err
	}

	SetDefaults(appConfig)

	return appConfig, nil
}

// SetDefaults sets the defaults for the provided object.
func SetDefaults(src runtime.Object) {
	Scheme.Default(src)
}
