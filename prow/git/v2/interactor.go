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

package git

// Interactor knows how to publish local work to a remote
type Interactor interface {
	Directory() string
	Clean() error
	MirrorClone() error
	Clone(from string) error
	Checkout(commitlike string) error
	RevParse(commitlike string) (string, error)
	BranchExists(branch string) bool
	CheckoutNewBranch(branch string) error
	Merge(commitlike string) (bool, error)
	MergeWithStrategy(commitlike, mergeStrategy string) (bool, error)
	MergeAndCheckout(baseSHA string, headSHAs []string, mergeStrategy string) error
	Am(path string) error
	Fetch() error
	RemoteUpdate() error
	FetchRef(refspec string) error
	CheckoutPullRequest(number int) error
	Config(key, value string) error
	Diff(head, sha string) (changes []string, err error)
	MergeCommitsExistBetween(target, head string) (bool, error)
}
