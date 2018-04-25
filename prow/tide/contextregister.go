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

package tide

import (
	"sync"

	"github.com/shurcooL/githubql"
	"k8s.io/apimachinery/pkg/util/sets"
)

type contextChecker interface {
	// ignoreContext tells whether a context is optional.
	ignoreContext(context Context) bool
	// missingRequiredContexts tells if required contexts are missing from the list of contexts provided.
	missingRequiredContexts([]Context) []Context
}

// newExpectedContext creates a Context with Expected state.
// This should not only be used when contexts are missing.
func newExpectedContext(c string) Context {
	return Context{
		Context:     githubql.String(c),
		State:       githubql.StatusStateExpected,
		Description: githubql.String(""),
	}
}

// contextRegister implements contextChecker and allow registering of required and optional contexts.
type contextRegister struct {
	lock               sync.RWMutex
	required, optional sets.String
}

// newContextRegister instantiates a new contextRegister and register the optional contexts provided.
func newContextRegister(optional ...string) *contextRegister {
	r := contextRegister{
		required: sets.NewString(),
		optional: sets.NewString(),
	}
	r.registerOptionalContexts(optional...)
	return &r
}

// ignoreContext checks whether a context can be ignored.
// Will return true if
// - context is registered as optional
// - required contexts are registered and the context provided is not required
// Will return false otherwise. Every context is required.
func (r *contextRegister) ignoreContext(c Context) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	if r.optional.Has(string(c.Context)) {
		return true
	}
	if r.required.Len() > 0 && !r.required.Has(string(c.Context)) {
		return true
	}
	return false
}

// missingRequiredContexts discard the optional contexts and only look of extra required contexts that are not provided.
func (r *contextRegister) missingRequiredContexts(contexts []Context) []Context {
	r.lock.RLock()
	defer r.lock.RUnlock()
	if r.required.Len() == 0 {
		return nil
	}
	existingContexts := sets.NewString()
	for _, c := range contexts {
		existingContexts.Insert(string(c.Context))
	}
	var missingContexts []Context
	for c := range r.required.Difference(existingContexts) {
		missingContexts = append(missingContexts, newExpectedContext(c))
	}
	return missingContexts
}

// registerOptionalContexts registers optional contexts
func (r *contextRegister) registerOptionalContexts(c ...string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.optional.Insert(c...)
}

// registerRequiredContexts register required contexts.
// Once required contexts are registered other contexts will be considered optional.
func (r *contextRegister) registerRequiredContexts(c ...string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.required.Insert(c...)
}
