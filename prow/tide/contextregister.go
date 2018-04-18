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

type ContextChecker interface {
	// IgnoreContext tells whether a context is optional.
	IgnoreContext(context Context) bool
	// MissingRequiredContexts tells if required contexts are missing from the list of contexts provided.
	MissingRequiredContexts([]Context) []Context
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

// ContextRegister implements ContextChecker and allow registering of required and optional contexts.
type ContextRegister struct {
	lock               sync.RWMutex
	required, optional sets.String
}

// NewContextRegister instantiates a new ContextRegister and register the optional contexts provided.
func NewContextRegister(optional ...string) *ContextRegister {
	r := ContextRegister{
		required: sets.NewString(),
		optional: sets.NewString(),
	}
	r.RegisterOptionalContexts(optional...)
	return &r
}

// IgnoreContext checks whether a context can be ignored.
// Will return true if
// - context is registered as optional
// - required contexts are registered and the context provided is not required
// Will return false otherwise. Every context is required.
func (r *ContextRegister) IgnoreContext(c Context) bool {
	if r.optional.Has(string(c.Context)) {
		return true
	}
	if r.required.Len() > 0 && !r.required.Has(string(c.Context)) {
		return true
	}
	return false
}

// MissingRequiredContexts discard the optional contexts and only look of extra required contexts that are not provided.
func (r *ContextRegister) MissingRequiredContexts(contexts []Context) []Context {
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

// RegisterOptionalContexts registers optional contexts
func (r *ContextRegister) RegisterOptionalContexts(c ...string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.optional.Insert(c...)
}

// RegisterRequiredContexts register required contexts.
// Once required contexts are registered other contexts will be considered optional.
func (r *ContextRegister) RegisterRequiredContexts(c ...string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.required.Insert(c...)
}
