/*
Copyright 2018 The Kubernetes Authors.

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

package userdashboard

import "net/http"

type GitOAuthAgent struct {
	gc *GitOAuthConfig
}


func NewGitOAuthAgent(config *GitOAuthConfig) (*GitOAuthAgent) {
	goaa := &GitOAuthAgent{
		gc: config,
	}

	goaa.registerCallBack()
	return goaa
}

func (goaa *GitOAuthAgent) registerCallBack() {
	mux := http.NewServeMux()

	// Required
	if config.CallbackURL {
		mux.Handle(config.CallbackURL, handleGitOAuthCallback())
	}

	// Optional
	if config.RedirectURI && validateRedirectURI(config.RedirectURI) {
		mux.Handle(config.RedirectURI, handleGitOAuthRedirect())
	}
}

func (goaa *GitOAuthAgent) PullRequest() {

}
