/*
Copyright 2020 The Kubernetes Authors.

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

package deployer

import (
	"testing"
)

func TestVerifyUpFlags(t *testing.T) {
	cases := []struct {
		name string

		deployer  deployer
		shouldErr bool
	}{
		{
			name: "0 num nodes",
			deployer: deployer{
				NumNodes: 0,
			},
			shouldErr: true,
		},
		{
			name: "-3 num nodes",
			deployer: deployer{
				NumNodes: -3,
			},
			shouldErr: true,
		},
		{
			name: "3 num nodes",
			deployer: deployer{
				NumNodes: 3,
			},
			shouldErr: false,
		},
	}

	for i := range cases {
		c := &cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			d := &c.deployer
			err := d.verifyUpFlags()
			if err != nil && !c.shouldErr {
				t.Errorf("got err when none was expected: %s", err)
			} else if err == nil && c.shouldErr {
				t.Error("got no error when one was expected")
			}
		})
	}
}
