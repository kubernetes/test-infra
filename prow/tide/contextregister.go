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
	"github.com/shurcooL/githubql"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
)

type contextChecker interface {
	// isOptional tells whether a context is optional.
	isOptional(Context) bool
	// missingRequiredContexts tells if required contexts are missing from the list of contexts provided.
	missingRequiredContexts([]Context) []Context
}

// newExpectedContext creates a Context with Expected state.
func newExpectedContext(c string) Context {
	return Context{
		Context:     githubql.String(c),
		State:       githubql.StatusStateExpected,
		Description: githubql.String(""),
	}
}

// contextRegister implements contextChecker and allow registering of required and optional contexts.
type contextRegister struct {
	required, optional  sets.String
	skipUnknownContexts bool
}

// newContextRegister instantiates a new contextRegister and register tide as optional by default
// and uses Prow Config to find optional and required tests, as well as skipUnknownContexts.
func newContextRegister(skipUnknownContexts bool) *contextRegister {
	r := &contextRegister{
		required:            sets.NewString(),
		optional:            sets.NewString(),
		skipUnknownContexts: skipUnknownContexts,
	}
	r.registerOptionalContexts(statusContext)
	return r
}

// newContextRegisterFromTideContextPolicy instantiates a new contextRegister and register tide as optional by default
// and uses Prow Config to find optional and required tests, as well as skipUnknownContexts.
func newContextRegisterFromTideContextPolicy(policy config.TideContextPolicy) *contextRegister {
	r := newContextRegister(false)
	if policy.SkipUnknownContexts != nil {
		r.skipUnknownContexts = *policy.SkipUnknownContexts
	}
	r.registerRequiredContexts(policy.RequiredContexts...)
	r.registerOptionalContexts(policy.OptionalContexts...)
	return r
}

// newContextRegisterFromTideContextPolicy instantiates a new contextRegister and register tide as optional by default
// and uses Prow Config to find optional and required tests, as well as skipUnknownContexts.
func newContextRegisterFromConfig(org, repo, branch string, c *config.Config) (*contextRegister, error) {
	policy, err := c.GetTideContextPolicy(org, repo, branch)
	if err != nil {
		return nil, err
	}
	r := newContextRegisterFromTideContextPolicy(policy)
	return r, nil
}

// isOptional checks whether a context can be ignored.
// Will return true if
// - context is registered as optional
// - required contexts are registered and the context provided is not required
// Will return false otherwise. Every context is required.
func (r *contextRegister) isOptional(c Context) bool {
	if r.optional.Has(string(c.Context)) {
		return true
	}
	if r.required.Has(string(c.Context)) {
		return false
	}
	if r.skipUnknownContexts {
		return true
	}
	return false
}

// missingRequiredContexts discard the optional contexts and only look of extra required contexts that are not provided.
func (r *contextRegister) missingRequiredContexts(contexts []Context) []Context {
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
	r.optional.Insert(c...)
}

// registerRequiredContexts register required contexts.
// Once required contexts are registered other contexts will be considered optional.
func (r *contextRegister) registerRequiredContexts(c ...string) {
	r.required.Insert(c...)
}
