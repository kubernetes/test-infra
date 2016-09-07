/*
Copyright 2016 The Kubernetes Authors.

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

package sql

import "testing"

func TestGetDSN(t *testing.T) {
	tests := []struct {
		config      MySQLConfig
		expectedDSN string
	}{
		{
			MySQLConfig{"localhost", 3306, "github", "root", "password"},
			"root:password@tcp(localhost:3306)/github?parseTime=True",
		},
		{
			MySQLConfig{"localhost", 3306, "github", "root", ""},
			"root@tcp(localhost:3306)/github?parseTime=True",
		},
		{
			MySQLConfig{"localhost", 3306, "", "root", ""},
			"root@tcp(localhost:3306)/?parseTime=True",
		},
	}

	for _, test := range tests {
		actualDSN := test.config.getDSN(test.config.Db)
		if actualDSN != test.expectedDSN {
			t.Error("Actual:", actualDSN, "doesn't match expected:", test.expectedDSN)
		}
	}
}
