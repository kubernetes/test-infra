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

package interface_types

type Zoo struct {
	// Animals is an array of animals of the zoo
	Animals []Animal `json:"animals"`
}

type Animal interface {
	// Sound the animal makes
	Sound()
}

type Cheetah struct {
	// Name of cheetah
	Name string `json:"name"`
}

func (c *Cheetah) Sound() {
	println("meowww")
}

type Lion struct {
	// Name of lion
	Name string `json:"name"`
}

func (l *Lion) Sound() {
	println("roarrr")
}
