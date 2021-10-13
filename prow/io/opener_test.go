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

package io

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
)

func Test_opener_SignedURL(t *testing.T) {
	// This fake key is revoked and thus worthless but still make its contents less obvious
	fakeKeyBuf, err := base64.StdEncoding.DecodeString(`
LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tXG5NSUlFdlFJQkFEQU5CZ2txaGtpRzl3MEJBUUVG
QUFTQ0JLY3dnZ1NqQWdFQUFvSUJBUUN4MEF2aW1yMjcwZDdaXG5pamw3b1FRUW1oZTFOb3dpeWMy
UStuQW95aFE1YkQvUW1jb01zcWg2YldneVI0UU90aXVBbHM2VWhJenF4Q25pXG5PazRmbWJqVnhp
STl1Ri9EVTV6ZE5wM0dkQWFiUlVPNW5yWkpMelN0VXhudFBEcjZvK281RHM5YWJJWkNYYUVTXG5o
UWxOdTBrUm5HbHZGUHNkV1JYMmtSN01Yb3pkcXczcHZZRXZyaGlhRStYZnRhUzhKdmZEc0NPT2RQ
OWp5TzNTXG5aR2lkaU5hRmhYK2xnZEcrdHdqOUE3UDFlb1NMbTZCdXVhcjRDOGhlOEVkVGVEbXVk
a1BPeWwvb2tHWU5tSzJkXG5yUkQ0WHBhcy93VGxsTXBLRUZxWllZeVdkRnJvVWQwMFVhQnhHV0cz
UlZ2TWZoRk80QUhrSkNwZlE1U00rSElmXG5VN2lkRjAyYkFnTUJBQUVDZ2dFQURIaVhoTTZ1bFFB
OHZZdzB5T2Q3cGdCd3ZqeHpxckwxc0gvb0l1dzlhK09jXG5QREMxRzV2aU5pZjdRVitEc3haeXlh
T0tISitKVktQcWZodnh3OFNmMHBxQlowdkpwNlR6SVE3R0ZSZXBLUFc4XG5NTVloYWRPZVFiUE00
emN3dWNpS1VuTW45dU1hcllmc2xxUnZDUjBrSEZDWWtucHB2RjYxckNQMGdZZjJJRXZUXG5qNVlV
QWFrNDlVRDQyaUdEZnh2OGUzMGlMTmRRWE1iMHE3V2dyRGdxL0ttUHM2Q2dOaGRzME1uSlRFbUE5
YlFtXG52MHV0K2hUYWpXalcxVWNyUTBnM2JjNng1VWN2V1VjK1ZndUllVmxVcEgvM2dJNXVYZkxn
bTVQNThNa0s4UlhTXG5YYW92Rk05VkNNRFhTK25PWk1uSXoyNVd5QmhkNmdpVWs5UkJhc05Tb1FL
QmdRRGFxUXpyYWJUZEZNY1hwVlNnXG41TUpuNEcvSFVPWUxveVM5cE9UZi9qbFN1ZUYrNkt6RGJV
N1F6TC9wT1JtYjJldVdxdmpmZDVBaU1oUnY2Snk1XG41ZVNpa3dYRDZJeS9sZGh3QUdtMUZrZ1ZX
TXJ3ZHlqYjJpV2I2Um4rNXRBYjgwdzNEN2ZTWWhEWkxUOWJCNjdCXG4ybGxiOGFycEJRcndZUFFB
U2pUVUVYQnVJUUtCZ1FEUUxVemkrd0tHNEhLeko1NE1sQjFoR3cwSFZlWEV4T0pmXG53bS9IVjhl
aThDeHZLMTRoRXpCT3JXQi9aNlo4VFFxWnA0eENnYkNiY0hwY3pLRUxvcDA2K2hqa1N3ZkR2TUJZ
XG5mNnN6U2RSenNYVTI1NndmcG1hRjJ0TlJZZFpVblh2QWc5MFIrb1BFSjhrRHd4cjdiMGZmL3lu
b0UrWUx0ckowXG53dklad3Joc093S0JnQWVPbWlTMHRZeUNnRkwvNHNuZ3ZodEs5WElGQ0w1VU9C
dlp6Qk0xdlJOdjJ5eEFyRi9nXG5zajJqSmVyUWoyTUVpQkRmL2RQelZPYnBwaTByOCthMDNFOEdG
OGZxakpxK2VnbDg2aXBaQjhxOUU5NTFyOUxSXG5Xa1ZtTEFEVVIxTC8rSjFhakxiWHJzOWlzZkxh
ZEI2OUJpT1lXWmpPRk0reitocmNkYkR5blZraEFvR0FJbW42XG50ZU1zN2NNWTh3anZsY0MrZ3Br
SU5GZzgzYVIyajhJQzNIOWtYMGs0N3ovS0ZjbW9TTGxjcEhNc0VJeGozamJXXG5kd0FkZy9TNkpi
RW1SbGdoaWVoaVNRc21RM05ta0xxNlFJWkorcjR4VkZ4RUZnOWFEM0szVUZMT0xickRCSFpJXG5D
M3JRWVpMNkpnY1E1TlBtbTk4QXZIN2RucjRiRGpaVDgzSS9McFVDZ1lFQWttNXlvVUtZY0tXMVQz
R1hadUNIXG40SDNWVGVzZDZyb3pKWUhmTWVkNE9jQ3l1bnBIVmZmSmFCMFIxRjZ2MjFQaitCVWlW
WjBzU010RjEvTE1uQkc4XG5TQVlQUnVxOHVNUUdNQTFpdE1Hc2VhMmg1V2RhbXNGODhXRFd4VEoy
QXVnblJHNERsdmJLUDhPQmVLUFFKeDhEXG5RMzJ2SVpNUVkyV1hVMVhwUkMrNWs5RT1cbi0tLS0t
RU5EIFBSSVZBVEUgS0VZLS0tLS1cbgo=`)
	if err != nil {
		t.Fatalf("Failed to decode fake key: %v", err)
	}
	fakePrivateKey := strings.TrimSpace(string(fakeKeyBuf))
	type args struct {
		ctx  context.Context
		p    string
		opts SignedURLOptions
	}
	tests := []struct {
		name      string
		args      args
		fakeCreds string
		want      string
		contains  []string
		wantErr   bool
	}{
		{
			name: "anon auth works",
			args: args{
				p: "gs://foo/bar/stuff",
			},
			want: fmt.Sprintf("https://%s/foo/bar/stuff", GSAnonHost),
		},
		{
			name: "cookie auth works",
			args: args{
				p: "gs://foo/bar/stuff",
				opts: SignedURLOptions{
					UseGSCookieAuth: true,
				},
			},
			want: fmt.Sprintf("https://%s/foo/bar/stuff", GSCookieHost),
		},
		{
			name: "signed URLs work",
			args: args{
				p: "gs://foo/bar/stuff",
			},
			fakeCreds: `{
			  "type": "service_account",
			  "private_key": "` + fakePrivateKey + `",
			  "client_email": "fake-user@k8s.io"
			}`,
			contains: []string{
				"https://storage.googleapis.com/foo/bar/stuff?",
				"GoogleAccessId=fake-user%40k8s.io",
				"Signature=", // Do not particularly care about the Signature contents
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gcsCredentialsFile string
			if tt.fakeCreds != "" {
				fp, err := ioutil.TempFile("", "fake-creds")
				if err != nil {
					t.Fatalf("Failed to create fake creds: %v", err)
				}

				gcsCredentialsFile = fp.Name()
				defer os.Remove(gcsCredentialsFile)
				if _, err := fp.Write([]byte(tt.fakeCreds)); err != nil {
					t.Fatalf("Failed to write fake creds %s: %v", gcsCredentialsFile, err)
				}

				if err := fp.Close(); err != nil {
					t.Fatalf("Failed to close fake creds %s: %v", gcsCredentialsFile, err)
				}
			}
			o, _ := NewOpener(context.Background(), gcsCredentialsFile, "")
			got, err := o.SignedURL(tt.args.ctx, tt.args.p, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignedURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("SignedURL() got = %v, want %v", got, tt.want)
			}
			if len(tt.contains) > 0 {
				for _, contains := range tt.contains {
					if !strings.Contains(got, contains) {
						t.Errorf("SignedURL() got = %q, does not contain %q", got, contains)
					}
				}
			}
		})
	}
}

func TestIsNotExist(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		err         error
		expectMatch bool
	}{
		{
			name:        "Direct ErrNotExist",
			err:         os.ErrNotExist,
			expectMatch: true,
		},
		{
			name:        "Direct storage.ErrObjectNotExist",
			err:         storage.ErrObjectNotExist,
			expectMatch: true,
		},
		{
			name:        "Direct other",
			err:         errors.New("not workx"),
			expectMatch: false,
		},
		{
			name:        "Wrapped ErrNotExist",
			err:         fmt.Errorf("around: %w", os.ErrNotExist),
			expectMatch: true,
		},
		{
			name:        "Wrapped storage.ErrObjectNotExist",
			err:         fmt.Errorf("around: %w", storage.ErrObjectNotExist),
			expectMatch: true,
		},
		{
			name:        "Wrapped other",
			err:         fmt.Errorf("I see it %w", errors.New("not workx")),
			expectMatch: false,
		},
		{
			name:        "Don't panic",
			expectMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsNotExist(tc.err); result != tc.expectMatch {
				t.Errorf("expect match: %t, got match: %t", tc.expectMatch, result)
			}
		})
	}
}
