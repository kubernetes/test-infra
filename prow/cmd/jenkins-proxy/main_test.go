package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

var _ Proxy = &fakeProxy{}

type fakeProxy struct {
	auth *BasicAuthConfig

	destURL string
	destErr error

	masters []string

	respErr error
}

func (fp *fakeProxy) Auth() *BasicAuthConfig { return fp.auth }
func (fp *fakeProxy) GetDestinationURL(r *http.Request, requestedJob string) (string, error) {
	return fp.destURL, fp.destErr
}
func (fp *fakeProxy) ProxyRequest(r *http.Request, destURL string) (*http.Response, error) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<xml>job config</xml>")
	}

	w := httptest.NewRecorder()
	handler(w, r)

	if fp.respErr != nil {
		return nil, fp.respErr
	}
	return w.Result(), nil
}
func (fp *fakeProxy) ListQueues(r *http.Request) (*http.Response, error) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		for _, m := range fp.masters {
			io.WriteString(w, fmt.Sprintf(`{"%s": "%s"}`, r.URL.RawQuery, m))
		}
	}

	w := httptest.NewRecorder()
	handler(w, r)

	if fp.respErr != nil {
		return nil, fp.respErr
	}
	return w.Result(), nil
}

func TestHandle(t *testing.T) {
	tests := []struct {
		name string

		method string
		url    string
		header *http.Header
		w      *httptest.ResponseRecorder
		p      *fakeProxy

		expectedBody string
	}{
		{
			name: "missing authentication",

			method: http.MethodGet,
			url:    "/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			w:      httptest.NewRecorder(),
			p: &fakeProxy{
				auth: &BasicAuthConfig{
					User:  "openshift-ci-robot",
					Token: "1234567890",
				},
			},

			expectedBody: "basic authentication required\n",
		},
		{
			name: "failed authentication",

			method: http.MethodGet,
			url:    "/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			w:      httptest.NewRecorder(),
			header: &http.Header{
				"Authorization": []string{"Basic b3BlbnNoaWZ0LWNpLXJvYm90OjEyMzQ1Njc4OQo="},
			},
			p: &fakeProxy{
				auth: &BasicAuthConfig{
					User:  "openshift-ci-robot",
					Token: "1234567890",
				},
			},

			expectedBody: "basic authentication failed\n",
		},
		{
			name: "successful authentication, job exists",

			method: http.MethodGet,
			url:    "/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			w:      httptest.NewRecorder(),
			header: &http.Header{
				"Authorization": []string{"Basic b3BlbnNoaWZ0LWNpLXJvYm90OjEyMzQ1Njc4OTA="},
			},
			p: &fakeProxy{
				auth: &BasicAuthConfig{
					User:  "openshift-ci-robot",
					Token: "1234567890",
				},
				destURL: "https://ci.openshift.redhat.com/jenkins/job/test_job/api/json",
			},

			expectedBody: "<xml>job config</xml>",
		},
		{
			name: "unauthenticated, job exists",

			w: httptest.NewRecorder(),
			p: &fakeProxy{
				destURL: "https://ci.openshift.redhat.com/jenkins/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			},
			method: http.MethodGet,
			url:    "/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",

			expectedBody: "<xml>job config</xml>",
		},
		{
			name: "job does not exist",

			method: http.MethodGet,
			url:    "/job/test_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			w:      httptest.NewRecorder(),
			p: &fakeProxy{
				destURL: "",
			},

			expectedBody: "404 page not found\n",
		},
		{
			name: "queue request",

			method: http.MethodGet,
			url:    "/queue/api/json?task",
			w:      httptest.NewRecorder(),
			p: &fakeProxy{
				masters: []string{"https://jenkins", "https://new-jenkins"},
			},

			expectedBody: `{"task": "https://jenkins"}{"task": "https://new-jenkins"}`,
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)
		req := httptest.NewRequest(test.method, test.url, nil)
		if test.header != nil {
			req.Header = *test.header
		}
		handle(test.p, test.w, req)
		if test.expectedBody != test.w.Body.String() {
			t.Errorf("expected body: %s, got: %s", test.expectedBody, test.w.Body.String())
		}
	}
}
