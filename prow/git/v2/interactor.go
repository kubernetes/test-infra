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

// Interactor knows how to operate on a git repository cloned from GitHub
// using a local cache.
type Interactor interface {
	// Directory exposes the directory in which the repository has been cloned
	Directory() string
	// Clean removes the repository. It is up to the user to call this once they are done
	Clean() error
	// Checkout runs `git checkout`
	Checkout(commitlike string) error
	// RevParse runs `git rev-parse`
	RevParse(commitlike string) (string, error)
	// BranchExists determines if a branch with the name exists
	BranchExists(branch string) bool
	// CheckoutNewBranch creates a new branch from HEAD and checks it out
	CheckoutNewBranch(branch string) error
	// Merge merges the commitlike into the current HEAD
	Merge(commitlike string) (bool, error)
	// MergeWithStrategy merges the commitlike into the current HEAD with the strategy
	MergeWithStrategy(commitlike, mergeStrategy string) (bool, error)
	// MergeAndCheckout merges all commitlikes into the current HEAD with the appropriate strategy
	MergeAndCheckout(baseSHA string, mergeStrategy string, headSHAs ...string) error
	// Am calls `git am`
	Am(path string) error
	// Fetch calls `git fetch`
	Fetch() error
	// FetchRef fetches the refspec
	FetchRef(refspec string) error
	// CheckoutPullRequest fetches and checks out the synthetic refspec from GitHub for a pull request HEAD
	CheckoutPullRequest(number int) error
	// Config runs `git config`
	Config(key, value string) error
	// Diff runs `git diff`
	Diff(head, sha string) (changes []string, err error)
	// MergeCommitsExistBetween determines if merge commits exist between target and HEAD
	MergeCommitsExistBetween(target, head string) (bool, error)
}
