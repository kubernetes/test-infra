/*
Copyright 2017 The Kubernetes Authors.

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

// Package jenkins includes a client and the operational logic for
// managing Jenkins masters in prow. It has been used with the
// following versions of Jenkins:
//
// * 2.60.3
// * 2.73.2
//
// It should most likely work for all versions but use at your own
// risk with a different version.
package jenkins
