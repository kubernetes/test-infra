/*
Copyright 2022 The Kubernetes Authors.

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

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"k8s.io/test-infra/prow/flagutil"
)

func TestCreateSecret(t *testing.T) {
	tests := []struct {
		name     string
		dnsNames []string
		expiry   int
		want     [3]string
	}{
		{
			name:     "base",
			dnsNames: []string{"bar"},
			expiry:   10,
			want:     [3]string{"bar10a", "bar10b", "bar10c"},
		},
		{
			name:     "noDnsNames",
			dnsNames: []string{},
			expiry:   10,
			want:     [3]string{"", "", ""},
		},
	}

	oldGenCertFunc := genCertFunc
	genCertFunc = func(expiry int, dnsNames []string) (string, string, string, error) {
		if len(dnsNames) == 0 {
			return "", "", "", errors.New("dnsNames was not configured")
		}
		baseName := dnsNames[0] + strconv.Itoa(expiry)
		return fmt.Sprintf("%s%s", baseName, "a"), fmt.Sprintf("%s%s", baseName, "b"), fmt.Sprintf("%s%s", baseName, "c"), nil
	}
	t.Cleanup(func() {
		genCertFunc = oldGenCertFunc
	})

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// t.Parallel()
			ctx := context.Background()
			clientoptions := clientOptions{
				secretID:      secretID,
				dnsNames:      flagutil.NewStrings(tc.dnsNames...),
				expiryInYears: tc.expiry,
			}
			f := newFakeClient()

			gotCert, gotPrivKey, gotCaPem, gotErr := createSecret(f, ctx, clientoptions)
			if tc.name == "noDnsNames" && gotErr == nil {
				t.Fatalf("unexpected error %v", gotErr)
			} else if tc.name != "noDnsNames" && gotErr != nil {
				t.Fatalf("unexpected error %v", gotErr)
			}
			if gotCert != tc.want[0] || gotPrivKey != tc.want[1] || gotCaPem != tc.want[2] {
				t.Fatalf("cert values do not match")
			}
		})
	}

}

func TestUpdateSecret(t *testing.T) {
	f := newFakeClient()
	expectedValue := "{\"caBundle.pem\":\"bar10c\",\"certFile.pem\":\"bar10a\",\"privKeyFile.pem\":\"bar10b\"}"
	f.project.store = map[string]string{secretID: "hello"}
	tests := []struct {
		name     string
		dnsNames []string
		expiry   int
		want     [3]string
	}{
		{
			name:     "base",
			dnsNames: []string{"bar"},
			expiry:   10,
			want:     [3]string{"bar10a", "bar10b", "bar10c"},
		},
		{
			name:     "noDnsNames",
			dnsNames: []string{},
			expiry:   10,
			want:     [3]string{"", "", ""},
		},
	}

	oldGenCertFunc := genCertFunc
	genCertFunc = func(expiry int, dnsNames []string) (string, string, string, error) {
		if len(dnsNames) == 0 {
			return "", "", "", errors.New("dnsNames was not configured")
		}
		baseName := dnsNames[0] + strconv.Itoa(expiry)
		return fmt.Sprintf("%s%s", baseName, "a"), fmt.Sprintf("%s%s", baseName, "b"), fmt.Sprintf("%s%s", baseName, "c"), nil
	}
	t.Cleanup(func() {
		genCertFunc = oldGenCertFunc
	})

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			clientoptions := clientOptions{
				secretID:      secretID,
				dnsNames:      flagutil.NewStrings(tc.dnsNames...),
				expiryInYears: tc.expiry,
			}

			gotCert, gotPrivKey, gotCaPem, gotErr := updateSecret(f, ctx, clientoptions)
			if tc.name == "noDnsNames" && gotErr == nil {
				t.Fatalf("unexpected error %v", gotErr)
			} else if tc.name != "noDnsNames" && gotErr != nil {
				t.Fatalf("unexpected error %v", gotErr)
			}
			if gotCert != tc.want[0] || gotPrivKey != tc.want[1] || gotCaPem != tc.want[2] {
				t.Fatalf("cert values do not match")
			}
			if f.project.store[secretID] != expectedValue {
				t.Fatalf("secret was not updated")
			}
		})
	}
}
