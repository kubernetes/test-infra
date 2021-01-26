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

package jira

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Client interface {
	GetIssue(id string) (*jira.Issue, error)
	GetRemoteLinks(id string) ([]jira.RemoteLink, error)
	AddRemoteLink(id string, link *jira.RemoteLink) error
	JiraClient() *jira.Client
	JiraURL() string
}

type BasicAuthGenerator func() (username, password string)

type Options struct {
	BasicAuth BasicAuthGenerator
	LogFields logrus.Fields
}

type Option func(*Options)

func WithBasicAuth(basicAuth BasicAuthGenerator) Option {
	return func(o *Options) {
		o.BasicAuth = basicAuth
	}
}

func WithFields(fields logrus.Fields) Option {
	return func(o *Options) {
		o.LogFields = fields
	}
}

func NewClient(endpoint string, opts ...Option) (Client, error) {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}
	log := logrus.WithField("client", "jira")
	if len(o.LogFields) > 0 {
		log = log.WithFields(o.LogFields)
	}

	retryingClient := retryablehttp.NewClient()
	retryingClient.Logger = &retryableHTTPLogrusWrapper{log: log}

	if o.BasicAuth != nil {
		retryingClient.HTTPClient.Transport = &basicAuthRoundtripper{
			generator: o.BasicAuth,
			upstream:  retryingClient.HTTPClient.Transport,
		}
	}

	jiraClient, err := jira.NewClient(retryingClient.StandardClient(), endpoint)
	return &client{upstream: jiraClient, url: endpoint}, err
}

type client struct {
	url      string
	upstream *jira.Client
}

func (jc *client) JiraClient() *jira.Client {
	return jc.upstream
}

func (jc *client) GetIssue(id string) (*jira.Issue, error) {
	issue, response, err := jc.upstream.Issue.Get(id, &jira.GetQueryOptions{})
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return nil, NotFoundError{err}
		}
		return nil, JiraError(response, err)
	}

	return issue, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, NotFoundError{})
}

func NewNotFoundError(err error) error {
	return NotFoundError{err}
}

type NotFoundError struct {
	error
}

func (NotFoundError) Is(target error) bool {
	_, match := target.(NotFoundError)
	return match
}

func (jc *client) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	result, resp, err := jc.upstream.Issue.GetRemoteLinks(id)
	if err != nil {
		return nil, JiraError(resp, err)
	}
	return *result, nil
}

func (jc *client) AddRemoteLink(id string, link *jira.RemoteLink) error {
	req, err := jc.upstream.NewRequest("POST", "rest/api/2/issue/"+id+"/remotelink", link)
	if err != nil {
		return fmt.Errorf("failed to construct request: %w", err)
	}
	resp, err := jc.upstream.Do(req, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("failed to add link: %w", JiraError(resp, err))
	}

	return nil
}

func (jc *client) JiraURL() string {
	return jc.url
}

type basicAuthRoundtripper struct {
	generator BasicAuthGenerator
	upstream  http.RoundTripper
}

func (bart *basicAuthRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := new(http.Request)
	*req2 = *req
	req2.URL = new(url.URL)
	*req2.URL = *req.URL
	user, pass := bart.generator()
	req2.SetBasicAuth(user, pass)
	logrus.WithField("curl", toCurl(req2)).Trace("Executing http request")
	return bart.upstream.RoundTrip(req2)
}

var knownAuthTypes = sets.NewString("bearer", "basic", "negotiate")

// maskAuthorizationHeader masks credential content from authorization headers
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
func maskAuthorizationHeader(key string, value string) string {
	if !strings.EqualFold(key, "Authorization") {
		return value
	}
	if len(value) == 0 {
		return ""
	}
	var authType string
	if i := strings.Index(value, " "); i > 0 {
		authType = value[0:i]
	} else {
		authType = value
	}
	if !knownAuthTypes.Has(strings.ToLower(authType)) {
		return "<masked>"
	}
	if len(value) > len(authType)+1 {
		value = authType + " <masked>"
	} else {
		value = authType
	}
	return value
}

// JiraError collapses cryptic Jira errors to include response
// bodies if it's detected that the original error holds no
// useful context in the first place
func JiraError(response *jira.Response, err error) error {
	if err != nil && strings.Contains(err.Error(), "Please analyze the request body for more details.") {
		if response != nil && response.Response != nil {
			body, readError := ioutil.ReadAll(response.Body)
			if readError != nil && readError.Error() != "http: read on closed response body" {
				logrus.WithError(readError).Warn("Failed to read Jira response body.")
			}
			return fmt.Errorf("%w: %v", err, string(body))
		}
	}
	return err
}

// toCurl is a slightly adjusted copy of https://github.com/kubernetes/kubernetes/blob/74053d555d71a14e3853b97e204d7d6415521375/staging/src/k8s.io/client-go/transport/round_trippers.go#L339
func toCurl(r *http.Request) string {
	headers := ""
	for key, values := range r.Header {
		for _, value := range values {
			headers += fmt.Sprintf(` -H %q`, fmt.Sprintf("%s: %s", key, maskAuthorizationHeader(key, value)))
		}
	}

	return fmt.Sprintf("curl -k -v -X%s %s '%s'", r.Method, headers, r.URL.String())
}

type retryableHTTPLogrusWrapper struct {
	log *logrus.Entry
}

// fieldsForContext translates a list of context fields to a
// logrus format; any items that don't conform to our expectations
// are omitted
func (l *retryableHTTPLogrusWrapper) fieldsForContext(context ...interface{}) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(context)-1; i += 2 {
		key, ok := context[i].(string)
		if !ok {
			continue
		}
		fields[key] = context[i+1]
	}
	return fields
}

func (l *retryableHTTPLogrusWrapper) Error(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Error(msg)
}

func (l *retryableHTTPLogrusWrapper) Info(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Info(msg)
}

func (l *retryableHTTPLogrusWrapper) Debug(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Debug(msg)
}

func (l *retryableHTTPLogrusWrapper) Warn(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Warn(msg)
}
