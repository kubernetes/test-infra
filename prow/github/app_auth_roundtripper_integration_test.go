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

package github

import (
	"crypto/rsa"
	"io/ioutil"
	"os"
	"testing"

	jwt "github.com/dgrijalva/jwt-go/v4"
	"github.com/sirupsen/logrus"
)

func TestGetOrg(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)
	appID := os.Getenv("APP_ID")
	privateKeyPath := os.Getenv("APP_PRIVATE_KEY_PATH")
	org := os.Getenv("APP_GITHUB_ORG")
	if appID == "" || privateKeyPath == "" || org == "" {
		t.SkipNow()
	}
	keyData, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		t.Fatalf("Failed to read private key: %v", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		t.Fatalf("Failed to parse key: %v", err)
	}

	_, client := NewAppsAuthClientWithFields(
		logrus.Fields{},
		func(b []byte) []byte { return b },
		appID,
		func() *rsa.PrivateKey { return key },
		"https://api.github.com/graphql",
		"http://localhost:8888",
	)

	if _, err := client.GetOrg(org); err != nil {
		t.Errorf("Failed to get org: %v", err)
	}
}
