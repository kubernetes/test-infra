/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package mungerutil

import "github.com/google/go-github/github"

// FilterValid filters a list of user data and returns only user data that contains valid username.
func FilterValid(users []*github.User) (res []*github.User) {
	for _, u := range users {
		if u != nil && u.Login != nil {
			res = append(res, u)
		}
	}
	return res
}
