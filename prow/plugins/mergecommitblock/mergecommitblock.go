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

// Package mergecommitblock adds a do-not-merge label to pull requests which contain merge commits
// Merge commits are defined as commits that contain more than one parent commit SHA

package mergecommitblock

const pluginName = "block-mc"
// init registers out plugin as a pull request handler
func init(){

}
// helpProvider provides information on the plugin
func helpProvider(){

}
// handlePullRequest returns the result of handlePR
func handlePullRequest(){

}
// githubClient defines what *github.Client methods we can use
type githubClient interface{

}
// handlePR takes a github client, a pull request event and applies, or removes applicable labels
func handlePR(){
		// Store all info about the owner, repo, num, and base sha of pull request
		// Use github client to get the commits in the pull request
		// Iterate through them and check for parent commits
		// If a commit is identified as a merge commit, store it somewhere
		// Once finished iterating, Label if merge commits were identified, and report back to end user what commits are merge commits
}
// TODO : (alisondy) Identify Usage
// isPRChanged  takes a github Pull request event and returns a boolean value, which indicates if code diffs have changed
func isPRChanged(){

}