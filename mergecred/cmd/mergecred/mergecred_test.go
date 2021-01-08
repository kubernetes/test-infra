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

package mergecred

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	fakesecretmanager "k8s.io/test-infra/mergecred/pkg/secretmanager/fake"
)

func TestValidateFlags(t *testing.T) {
	tmpFileInfo, err := ioutil.TempFile(".", "mergecred-tmp-file")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile := tmpFileInfo.Name()
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})
	tests := []struct {
		name        string
		args        []string
		errExpected bool
	}{
		{
			name: "valid",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: false,
		}, {
			name: "project is required",
			args: []string{
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: true,
		}, {
			name: "context is required",
			args: []string{
				"--project=project",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: true,
		}, {
			name: "kubeconfig-to-merge must be a valid path",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				"--kubeconfig-to-merge=/dev/null",
			},
			errExpected: true,
		}, {
			name: "kubeconfig-to-merge must be a file",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				"--kubeconfig-to-merge=tmp",
			},
			errExpected: true,
		}, {
			name: "auto and dst-key exclusive",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--dst-key=dst-key",
			},
			errExpected: true,
		}, {
			name: "src-key is required when auto is off",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--auto=false",
				"--dst-key=dst-key",
			},
			errExpected: true,
		}, {
			name: "dst-key is required when auto is off",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--auto=false",
				"--src-key=src-key",
			},
			errExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var o options

			os.Args = []string{"mergecred"}
			pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
			os.Args = append(os.Args, test.args...)
			o.parseFlags()

			if hasErr := o.validateFlags() != nil; hasErr != test.errExpected {
				t.Errorf("expected err: %t but was %t", test.errExpected, hasErr)
			}
		})
	}
}

func TestGetKeys(t *testing.T) {
	tests := []struct {
		name       string
		secretMap  map[string][]byte
		o          options
		wantSrcKey string
		wantDstKey string
		wantPrune  bool
		wantErr    bool
	}{
		{
			name: "auto",
			secretMap: map[string][]byte{
				"config-20000101": []byte("secret1"),
			},
			o: options{
				auto: true,
			},
			wantSrcKey: "config-20000101",
			wantDstKey: fmt.Sprintf("config-%s", time.Now().Format("20060102")),
			wantPrune:  true,
			wantErr:    false,
		}, {
			name: "multiple keys",
			secretMap: map[string][]byte{
				"config-20000101": []byte("secret1"),
				"config-20000102": []byte("secret2"),
			},
			o: options{
				auto: true,
			},
			wantSrcKey: "config-20000102",
			wantDstKey: fmt.Sprintf("config-%s", time.Now().Format("20060102")),
			wantPrune:  true,
			wantErr:    false,
		}, {
			name: "invalid secretmap",
			secretMap: map[string][]byte{
				"config-200001012": []byte("secret1"),
			},
			o: options{
				auto: true,
			},
			wantSrcKey: "",
			wantDstKey: "",
			wantPrune:  false,
			wantErr:    true,
		}, {
			name: "manual",
			secretMap: map[string][]byte{
				"config-20000101": []byte("secret1"),
			},
			o: options{
				auto:   false,
				srcKey: "manual-src",
				dstKey: "manual-dst",
				prune:  false,
			},
			wantSrcKey: "manual-src",
			wantDstKey: "manual-dst",
			wantPrune:  false,
			wantErr:    false,
		},
	}

	assert := func(want, got interface{}) {
		if want != got {
			log.Fatalf("Mismatch, want: %v, got: %v", want, got)
		}
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			getSrcKey, getDstKey, getPrune, getErr := getKeys(tt.secretMap, tt.o)
			getErrBool := (getErr != nil)
			assert(tt.wantSrcKey, getSrcKey)
			assert(tt.wantDstKey, getDstKey)
			assert(tt.wantPrune, getPrune)
			assert(tt.wantErr, getErrBool)
		})
	}
}

func TestBackupSecret(t *testing.T) {
	fakeSecretID := "fake-secret"
	fakeVal := "fakeval"
	tests := []struct {
		name          string
		secrets       map[string]*secretmanagerpb.Secret
		versions      map[string]map[string][]byte
		listErr       error
		createErr     error
		addVersionErr error
		expSecret     []string
		expVersion    map[string]map[string][]byte
		expError      bool
	}{
		{
			name:          "new",
			secrets:       make(map[string]*secretmanagerpb.Secret),
			versions:      make(map[string]map[string][]byte),
			listErr:       nil,
			createErr:     nil,
			addVersionErr: nil,
			expSecret:     []string{fakeSecretID},
			expVersion: map[string]map[string][]byte{
				fakeSecretID: map[string][]byte{
					"0": []byte(fakeVal),
				},
			},
			expError: false,
		}, {
			name: "exist",
			secrets: map[string]*secretmanagerpb.Secret{
				fakeSecretID: &secretmanagerpb.Secret{Name: fmt.Sprintf("projects/%s/secrets/%s", "", fakeSecretID)},
			},
			versions: map[string]map[string][]byte{
				fakeSecretID: map[string][]byte{
					"0": []byte("somerandomvalue"),
				},
			},
			listErr:       nil,
			createErr:     nil,
			addVersionErr: nil,
			expSecret:     []string{fakeSecretID},
			expVersion: map[string]map[string][]byte{
				fakeSecretID: map[string][]byte{
					"1": []byte(fakeVal),
				},
			},
			expError: false,
		}, {
			name:          "list error",
			secrets:       make(map[string]*secretmanagerpb.Secret),
			versions:      make(map[string]map[string][]byte),
			listErr:       errors.New("fake list error"),
			createErr:     nil,
			addVersionErr: nil,
			expSecret:     nil,
			expVersion:    nil,
			expError:      true,
		}, {
			name:          "create error",
			secrets:       make(map[string]*secretmanagerpb.Secret),
			versions:      make(map[string]map[string][]byte),
			listErr:       nil,
			createErr:     errors.New("fake create secret error"),
			addVersionErr: nil,
			expSecret:     nil,
			expVersion:    nil,
			expError:      true,
		}, {
			name:          "add version error",
			secrets:       make(map[string]*secretmanagerpb.Secret),
			versions:      make(map[string]map[string][]byte),
			listErr:       nil,
			createErr:     nil,
			addVersionErr: errors.New("fake add version error"),
			expSecret:     nil,
			expVersion:    nil,
			expError:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fc := fakesecretmanager.NewFakeClient()
			fc.SecretMap = tt.secrets
			fc.SecretVersion = tt.versions
			fc.PreloadedErrors = []error{tt.listErr, tt.createErr, tt.addVersionErr}
			gotErr := backupSecret(ctx, fc, fakeSecretID, []byte(fakeVal))
			if want, got := tt.expError, (gotErr != nil); want != got {
				t.Fatalf("Error mismatch, want: %v, got: %v", want, gotErr)
			}
			for _, s := range tt.expSecret {
				if _, ok := fc.SecretMap[s]; !ok {
					t.Fatalf("Secret %s expect to exist", s)
				}
			}
			for wantSecretID, wantVersions := range tt.expVersion {
				gotVerions, ok := fc.SecretVersion[wantSecretID]
				if !ok {
					t.Fatalf("Secret %s expect to have version, but not", wantSecretID)
				}
				for wantName, wantVal := range wantVersions {
					gotVal, ok := gotVerions[wantName]
					if !ok {
						t.Fatalf("Version %s/%s expects to exist, but not", wantSecretID, wantName)
					}
					if diff := cmp.Diff(string(wantVal), string(gotVal)); diff != "" {
						t.Fatalf("Secret version value mismatch. want(-), got(+): %v", diff)
					}
				}
			}
		})
	}
}
