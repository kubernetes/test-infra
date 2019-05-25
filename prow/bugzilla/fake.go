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

package bugzilla

import (
	"errors"
	"net/http"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Fake is a fake Bugzilla client with injectable fields
type Fake struct {
	EndpointString string
	Bugs           map[int]Bug
	BugErrors      sets.Int
}

// Endpoint returns the endpoint for this fake
func (c *Fake) Endpoint() string {
	return c.EndpointString
}

// GetBug retrieves the bug, if registered, or an error, if set,
// or responds with an error that matches IsNotFound
func (c *Fake) GetBug(id int) (*Bug, error) {
	if c.BugErrors.Has(id) {
		return nil, errors.New("injected error getting bug")
	}
	if bug, exists := c.Bugs[id]; exists {
		return &bug, nil
	} else {
		return nil, &requestError{statusCode: http.StatusNotFound, message: "bug not registered in the fake"}
	}
}

// the Fake is a Client
var _ Client = &Fake{}
