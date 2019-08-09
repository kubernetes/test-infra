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

package resources

type Interface interface {
	// ARN returns the AWS ARN for the resource
	// (c.f. http://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html). This
	// is only used for uniqueness in the Mark set, but ARNs are
	// intended to be globally unique across regions and accounts, so
	// that works.
	ARN() string

	// ResourceKey() returns a per-resource key, because ARNs might conflict if two objects
	// with the same name are created at different times (e.g. IAM roles)
	ResourceKey() string
}
