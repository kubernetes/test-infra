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

package gcs

import (
	"io"
	"io/ioutil"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	storage "google.golang.org/api/storage/v1"
)

const (
	scope = storage.DevstorageFullControlScope
)

type Client struct {
	service *storage.Service
}

// NewClient creates a client from within the cluster.
func NewClient() (*Client, error) {
	c, err := google.DefaultClient(context.Background(), scope)
	if err != nil {
		return nil, err
	}
	service, err := storage.New(c)
	if err != nil {
		return nil, err
	}
	return &Client{
		service: service,
	}, nil
}

// Upload uploads everything from the reader into the given bucket/object name
// without verifying any generation numbers or anything like that.
func (c *Client) Upload(r io.Reader, bucket, name string) error {
	object := &storage.Object{Name: name}
	_, err := c.service.Objects.Insert(bucket, object).Media(r, googleapi.ContentType("")).Do()
	return err
}

// Download downloads the object.
func (c *Client) Download(bucket, name string) ([]byte, error) {
	resp, err := c.service.Objects.Get(bucket, name).Download()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}
