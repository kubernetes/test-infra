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

package objectstorage

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"gocloud.dev/blob"
)

// ClientWithCreds returns a storage client, optionally authenticated with the specified .json creds
func ClientWithCreds(ctx context.Context, bucket string, creds ...string) (*blob.Bucket, error) {
	options := map[string]string{}
	options["bucket"] = bucket

	switch l := len(creds); l {
	case 0: // Do nothing
	case 1:
		options["creds"] = creds[0]
	default:
		return nil, fmt.Errorf("%d creds files unsupported (at most 1)", l)
	}

	u, err := url.Parse(bucket)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "s3":
		return setupAWS(ctx, options)
	case "gs":
		return setupGCP(ctx, options)
	case "file":
		return setupLocal(ctx, options)
	default:
		// Default to GCP for backward compatibility
		return setupGCP(ctx, options)
	}
}

// Path parses gs://bucket/obj urls
type Path struct {
	url url.URL
}

func NewPath(path string) (*Path, error) {
	var p Path
	err := p.Set(path)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// String returns the gs://bucket/obj url
func (g Path) String() string {
	return g.url.String()
}

// Set updates value from a gs://bucket/obj string, validating errors.
func (g *Path) Set(v string) error {
	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("invalid bucket url %s: %v", v, err)
	}
	return g.SetURL(u)
}

// SetURL updates value to the passed in gs://bucket/obj url
func (g *Path) SetURL(u *url.URL) error {
	var scheme = regexp.MustCompile(`gs|s3`)

	switch {
	case u == nil:
		return errors.New("nil url")
	case !scheme.MatchString(u.Scheme):
		return fmt.Errorf("must use a gs:// or s3:// url: %s", u)
	case strings.Contains(u.Host, ":"):
		return fmt.Errorf("bucket may not contain a port: %s", u)
	case u.Opaque != "":
		return fmt.Errorf("url must start with gs:// or s3://: %s", u)
	case u.User != nil:
		return fmt.Errorf("bucket may not contain an user@ prefix: %s", u)
	case u.RawQuery != "":
		return fmt.Errorf("bucket may not contain a ?query suffix: %s", u)
	case u.Fragment != "":
		return fmt.Errorf("bucket may not contain a #fragment suffix: %s", u)
	}
	g.url = *u
	return nil
}

// ResolveReference returns the path relative to the current path
func (g Path) ResolveReference(ref *url.URL) (*Path, error) {
	var newP Path
	if err := newP.SetURL(g.url.ResolveReference(ref)); err != nil {
		return nil, err
	}
	return &newP, nil
}

// Bucket returns bucket in gs://bucket/obj
func (g Path) Bucket() string {
	return g.url.Host
}

// Object returns path/to/something in gs://bucket/path/to/something
func (g Path) Object() string {
	if g.url.Path == "" {
		return g.url.Path
	}
	return g.url.Path[1:]
}

func calcMD5(buf []byte) []byte {
	sum := md5.Sum(buf)
	return sum[:]
}

const (
	// Default ACLs for this upload
	Default = false
	// PublicRead ACL for this upload.
	PublicRead = true
)

// Upload writes bytes to the specified Path
func Upload(ctx context.Context, client *blob.Bucket, path Path, buf []byte, worldReadable bool) error {
	md5Sum := calcMD5(buf)

	opts := &blob.WriterOptions{ContentMD5: md5Sum}
	w, err := client.NewWriter(ctx, path.Object(), opts)
	if err != nil {
		return fmt.Errorf("Creating new writer failed: %v", err)
	}
	_, err = io.Copy(w, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("writing %s failed: %v", path, err)
	}
	err = w.Close()
	if err != nil {
		return fmt.Errorf("closing %s failed: %v", path, err)
	}
	return nil
}
