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

package embedded_structs

type Building struct {
	// Address of building
	Address string
	// Bathroom in building
	Bathroom `json:"bathroom"`
	// Bedroom in building
	Bedroom `json:"bedroom"`
}

type Bathroom struct {
	// Width of Bathroom
	Width int `json:"width"`
	// Height of Bathroom
	Height int `json:"height"`
}

type Bedroom struct {
	// Width of Bedroom
	Width int `json:"width"`
	// Height of Bedroom
	Height int `json:"height"`
}
