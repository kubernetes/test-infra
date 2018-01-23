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

import (
	"net/http"
	"fmt"
	"k8s.io/test-infra/testgrid/config/pb"
)

type UserData struct {

}

type DashboardAgent struct {
	goaa *GitOAuthAgent
}

func (da *DashboardAgent) NewDashboardAgent() (*DashboardAgent) {
	return nil
}

func (da *DashboardAgent) generateUserData() (*UserData) {
	da.goaa.PullRequest()
}

func (da *DashboardAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")

}