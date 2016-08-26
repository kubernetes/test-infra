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

package fakegcs

import (
	"io"
	"io/ioutil"
)

// FakeClient has the same methods as gcs.Client.
type FakeClient struct {
	// bucket -> object name -> data
	Objects map[string]map[string][]byte
}

func (c *FakeClient) Upload(r io.Reader, bucket, name string) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	if c.Objects == nil {
		c.Objects = make(map[string]map[string][]byte)
	}
	_, ok := c.Objects[bucket]
	if !ok {
		c.Objects[bucket] = map[string][]byte{}
	}
	c.Objects[bucket][name] = b
	return nil
}

func (c *FakeClient) Download(bucket, name string) ([]byte, error) {
	return c.Objects[bucket][name], nil
}
