/*
Copyright 2021 The Kubernetes Authors.

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

package ownersconfig

// Filenames configures which file names should be used for the OWNERS and OWNERS_ALIASES
// concepts for a repo, if it's not the default set.
type Filenames struct {
	Owners        string `json:"owners,omitempty"`
	OwnersAliases string `json:"owners_aliases,omitempty"`
}

const (
	DefaultOwnersFile        = "OWNERS"
	DefaultOwnersAliasesFile = "OWNERS_ALIASES"
)

type Resolver func(org, repo string) Filenames

// FakeResolver fills in for tests that use a resolver but aren't testing it.
// This should not be used in production code.
func FakeResolver(_, _ string) Filenames {
	return FakeFilenames
}

// FakeFilenames fills in for tests that need a Filenames but aren't testing them.
// While this *is* the default Filenames, production code should not use this var
// and instead expect to get the default set of filenames when using a resolver.
var FakeFilenames = Filenames{
	Owners:        DefaultOwnersFile,
	OwnersAliases: DefaultOwnersAliasesFile,
}
