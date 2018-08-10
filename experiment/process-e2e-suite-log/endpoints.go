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

package main

var endpoints = map[string]string{
	"Conformance": `\[Conformance\]`,

	"POST /api/v1/namespaces/{namespace}/pods":                 "create /api/v1/namespaces/.*/pods\n",
	"POST /api/v1/namespaces/{namespace}/pods/{name}/eviction": "create /api/v1/namespaces/.*/pods/.*/eviction\n",
	"PATCH /api/v1/namespaces/{namespace}/pods/{name}":         "patch /api/v1/namespaces/.*/pods/[^/]*\n",
	"PUT /api/v1/namespaces/{namespace}/pods/{name}":           "update /api/v1/namespaces/.*/pods/[^/]*\n",
	"DELETE /api/v1/namespaces/{namespace}/pods/{name}":        "delete /api/v1/namespaces/.*/pods/[^/]*\n",
	"DELETE /api/v1/namespaces/{namespace}/pods":               "deletecollection /api/v1/namespaces/.*/pods\n",

	"GET /api/v1/namespaces/{namespace}/pods/{name}":       "get /api/v1/namespaces/.*/pods/[^/]*\n",
	"GET /api/v1/namespaces/{namespace}/pods":              "list /api/v1/namespaces/.*/pods\n",
	"GET /api/v1/pods":                                     "list /api/v1/pods\n",
	"GET /api/v1/watch/namespaces/{namespace}/pods/{name}": "watch /api/v1/namespaces/.*/pods/[^/]*\n",
	"GET /api/v1/watch/namespaces/{namespace}/pods":        "watch /api/v1/namespaces/.*/pods\n",
	"GET /api/v1/watch/pods":                               "watch /api/v1/pods\n",

	"PATCH /api/v1/namespaces/{namespace}/pods/{name}/status": "patch /api/v1/namespaces/.*/pods/.*/status\n",
	"GET /api/v1/namespaces/{namespace}/pods/{name}/status":   "get /api/v1/namespaces/.*/pods/.*/status\n",
	"PUT /api/v1/namespaces/{namespace}/pods/{name}/status":   "update /api/v1/namespaces/.*/pods/.*/status\n",

	"POST /api/v1/namespaces/{namespace}/pods/{name}/portforward":    "create /api/v1/namespaces/.*/pods/.*/portforward\n",
	"POST /api/v1/namespaces/{namespace}/pods/{name}/proxy":          "create /api/v1/namespaces/.*/pods/.*/proxy\n",
	"POST /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}":   "create /api/v1/namespaces/.*/pods/.*/proxy/[^/]*\n",
	"DELETE /api/v1/namespaces/{namespace}/pods/{name}/proxy":        "delete /api/v1/namespaces/.*/pods/.*/proxy\n",
	"DELETE /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}": "delete /api/v1/namespaces/.*/pods/.*/proxy/[^/]*\n",
	"GET /api/v1/namespaces/{namespace}/pods/{name}/portforward":     "list /api/v1/namespaces/.*/pods/.*/portforward\n",
	"GET /api/v1/namespaces/{namespace}/pods/{name}/proxy":           "list /api/v1/namespaces/.*/pods/.*/proxy\n",
	"GET /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}":    "get /api/v1/namespaces/.*/pods/.*/proxy/[^/]*\n",

	"PUT /api/v1/namespaces/{namespace}/pods/{name}/proxy":        "update /api/v1/namespaces/.*/pods/.*/proxy\n",
	"PUT /api/v1/namespaces/{namespace}/pods/{name}/proxy/{path}": "update /api/v1/namespaces/.*/pods/.*/proxy/[^/]*\n",

	"GET /api/v1/namespaces/{namespace}/pods/{name}/log": "get /api/v1/namespaces/.*/pods/.*/log\n",
}
