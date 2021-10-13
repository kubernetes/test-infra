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

package apptokenequalizer

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

func New(delegate http.RoundTripper) http.RoundTripper {
	return &appTokenEqualizerTransport{
		delegate:   delegate,
		tokenCache: map[string]github.AppInstallationToken{},
	}
}

type appTokenEqualizerTransport struct {
	delegate   http.RoundTripper
	lock       sync.Mutex
	tokenCache map[string]github.AppInstallationToken
}

func (t *appTokenEqualizerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.delegate.RoundTrip(r)
	if err != nil || resp.StatusCode != 201 || r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/app/installations/") {
		return resp, err
	}
	body := resp.Body
	defer body.Close()

	l := logrus.WithField("path", r.URL.Path)
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		l.WithError(err).Error("Failed to read body")
		resp.StatusCode = http.StatusInternalServerError
		resp.Status = http.StatusText(http.StatusInternalServerError)
		return resp, nil
	}

	var token github.AppInstallationToken
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		l.WithError(err).Error("Failed to unmarshal")
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		return resp, nil
	}

	split := strings.Split(r.URL.Path, "/")
	if n := len(split); n != 5 {
		l.Errorf("Splitting path %s by '/' didn't yield exactly five elements but %d", r.URL.Path, n)
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		return resp, nil
	}
	appId := split[3]

	t.lock.Lock()
	defer t.lock.Unlock()
	if cachedToken, ok := t.tokenCache[appId]; ok && cachedToken.ExpiresAt.Add(-time.Minute).After(time.Now()) {
		token = cachedToken
	} else {
		t.tokenCache[appId] = token
	}

	serializedToken, err := json.Marshal(token)
	if err != nil {
		l.WithError(err).Error("Failed to serialize token")
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		return resp, nil
	}

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(serializedToken))
	resp.ContentLength = int64(len(serializedToken))
	resp.Header.Set("X-PROW-GHPROXY-REPLACED-TOKEN", "true")
	resp.Header.Set("Content-Length", strconv.Itoa(len(serializedToken)))
	return resp, nil
}
