package repoowners

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// TestEndpointSuccess tests the handling of normal input by the owners endpoint.
func TestEndpointSuccess(t *testing.T) {
	tests := []struct {
		name         string
		body         *ownersRequest
		contentType  string
		expectedBody string
		expectedCode int
	}{
		{
			name: "general use case",
			body: &ownersRequest{
				Org:        "org",
				Repo:       "repo",
				BaseCommit: "master",
				Paths: []string{
					"src/dir",
					"src",
					"foo",
					"src/dir/conformance",
					"docs",
					"docs/file.md",
				},
			},
			contentType:  "application/json",
			expectedBody: `{"docs":{"approvers":["cjwagner"],"reviewers":["alice","bob"],"required_reviewers":["chris"],"labels":["EVERYTHING"]},"docs/file.md":{"approvers":["alice","cjwagner"],"reviewers":["alice","bob"],"required_reviewers":["chris"],"labels":["EVERYTHING","docs"]},"foo":{"approvers":["cjwagner"],"reviewers":["alice","bob"],"required_reviewers":["chris"],"labels":["EVERYTHING"]},"src":{"approvers":["carl","cjwagner"],"reviewers":["alice","bob"],"required_reviewers":["chris"],"labels":["EVERYTHING"]},"src/dir":{"approvers":["bob","carl","cjwagner"],"reviewers":["alice","bob","cjwagner"],"required_reviewers":["ben","chris"],"labels":["EVERYTHING","src-code"]},"src/dir/conformance":{"approvers":["mml"]}}`,
			expectedCode: http.StatusOK,
		},
		{
			name: "Missing content-type information",
			body: &ownersRequest{
				Org:        "org",
				Repo:       "repo",
				BaseCommit: "master",
				Paths: []string{
					"src/dir",
					"src",
					"foo",
					"src/dir/conformance",
					"docs",
					"docs/file.md",
				},
			},
			contentType:  "",
			expectedBody: `{"error_message":"This JSON-API require a JSON content-type"}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "Empty paths",
			body: &ownersRequest{
				Org:        "org",
				Repo:       "repo",
				BaseCommit: "master",
				Paths:      []string{},
			},
			contentType:  "application/json",
			expectedBody: `{"error_message":"Path(s) list is empty or non-existing"}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "Empty body",
			body:         nil,
			contentType:  "application/json",
			expectedBody: `{"error_message":"Path(s) list is empty or non-existing"}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "Non-valid basecommit",
			body: &ownersRequest{
				Org:        "org",
				Repo:       "repo",
				BaseCommit: "13202CD5-3068-46D4-8C1D-46F544ED39EF",
				Paths: []string{
					"src/dir",
					"src",
					"foo",
					"src/dir/conformance",
					"docs",
					"docs/file.md",
				},
			},
			contentType:  "application/json",
			expectedBody: `{"error_message":"The Org or repo or BaseCommit may be not valid(s) : error checking out 13202CD5-3068-46D4-8C1D-46F544ED39EF: exit status 1. output: error: pathspec '13202CD5-3068-46D4-8C1D-46F544ED39EF' did not match any file(s) known to git\n"}`,
			expectedCode: http.StatusBadRequest,
		},
	}

	client, cleanup, initErr := getTestClient(testFiles, true, false, true, nil, nil, nil)
	switch {
	case initErr != nil:
		t.Fatalf("Error creating test client: %v.", initErr)
	case client == nil:
		t.Fatalf("Fake client could not be instantiated.")
	case cleanup == nil:
		t.Fatalf("Fake client cleaner could not be instantiated.")
	}
	defer cleanup()

	ownersServer := &OwnersServer{OwnersClient: client}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		rawBody, err := json.Marshal(test.body)
		if err != nil {
			t.Fatalf("Ill-formed testing input %v.", err)
		}

		req, err := http.NewRequest("POST", "/", bytes.NewBuffer(rawBody))
		if err != nil {
			t.Fatalf("POST query to the EndPoint failed : %v.", err)
		}
		req.Header.Set("Content-Type", test.contentType)

		rr := httptest.NewRecorder()
		ownersServer.ServeHTTP(rr, req)
		if rr.Code != test.expectedCode {
			t.Errorf("Handler returned wrong status code: got %v want %v", rr.Code, test.expectedCode)
		}
		if rr.Body.String() != test.expectedBody {
			t.Errorf("Handler returned unexpected body: got %v want %v", rr.Body.String(), test.expectedBody)
		}
	}
}

// TestEndPointRegex tests the handling of edge-cases inputs by the owners endpoint which need regex.
// We have to use regex regarding the fact we are grounded on RepoOwners error that can contain random-based path
func TestEndPointRegex(t *testing.T) {
	tests := []struct {
		name              string
		body              *ownersRequest
		contentType       string
		expectedBodyRegex string
		expectedCode      int
	}{
		{
			name: "Non-valid org",
			body: &ownersRequest{
				Org:        "9AA48AEB-30EB-4187-89E7-5FAEACC3EC91",
				Repo:       "repo",
				BaseCommit: "master",
				Paths: []string{
					"src/dir",
					"src",
					"foo",
					"src/dir/conformance",
					"docs",
					"docs/file.md",
				},
			},
			contentType:       "application/json",
			expectedBodyRegex: `^{"error_message":"The Org or repo or BaseCommit may be not valid\(s\) : failed to clone 9AA48AEB-30EB-4187-89E7-5FAEACC3EC91\/repo: git cache clone error: exit status 128\. output: fatal: repository .* does not exist\\n"}$`,
			expectedCode:      http.StatusBadRequest,
		},
		{
			name: "Non-valid repo",
			body: &ownersRequest{
				Org:        "org",
				Repo:       "A1A1693C-082C-41B1-9943-3A89B08A57C0",
				BaseCommit: "Master",
				Paths: []string{
					"src/dir",
					"src",
					"foo",
					"src/dir/conformance",
					"docs",
					"docs/file.md",
				},
			},
			contentType:       "application/json",
			expectedBodyRegex: `^{"error_message":"The Org or repo or BaseCommit may be not valid\(s\) : failed to clone org\/A1A1693C-082C-41B1-9943-3A89B08A57C0: git cache clone error: exit status 128\. output: fatal: repository .* does not exist\\n"}$`,
			expectedCode:      http.StatusBadRequest,
		},
	}

	client, cleanup, initErr := getTestClient(testFiles, true, false, true, nil, nil, nil)
	switch {
	case initErr != nil:
		t.Fatalf("Error creating test client: %v.", initErr)
	case client == nil:
		t.Fatalf("Fake client could not be instantiated.")
	case cleanup == nil:
		t.Fatalf("Fake client cleaner could not be instantiated.")
	}
	defer cleanup()

	ownersServer := &OwnersServer{OwnersClient: client}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		rawBody, err := json.Marshal(test.body)
		if err != nil {
			t.Fatalf("Ill-formed testing input %v.", err)
		}

		req, err := http.NewRequest("POST", "/", bytes.NewBuffer(rawBody))
		if err != nil {
			t.Fatalf("POST query to the EndPoint failed : %v.", err)
		}
		req.Header.Set("Content-Type", test.contentType)

		rr := httptest.NewRecorder()
		ownersServer.ServeHTTP(rr, req)
		if rr.Code != test.expectedCode {
			t.Errorf("Handler returned wrong status code: got %v want %v", rr.Code, test.expectedCode)
		}
		matched, err := regexp.MatchString(test.expectedBodyRegex, rr.Body.String())
		if matched != true {
			t.Errorf("Handler returned unexpected body: got %v want to match regex(%v)", rr.Body.String(), test.expectedBodyRegex)
		}
	}
}

// TestEndpointCorrupted tests the handling of corrupted messages by the owners endpoint.
func TestEndpointCorrupted(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		contentType  string
		expectedBody string
		expectedCode int
	}{
		{
			name:         "Empty body",
			body:         []byte(""),
			contentType:  "application/json",
			expectedBody: `{"error_message":"The body is empty, there is no content to parse"}`,
			expectedCode: http.StatusBadRequest,
		},
	}

	client, cleanup, initErr := getTestClient(testFiles, true, false, true, nil, nil, nil)
	switch {
	case initErr != nil:
		t.Fatalf("Error creating test client: %v.", initErr)
	case client == nil:
		t.Fatalf("Fake client could not be instantiated.")
	case cleanup == nil:
		t.Fatalf("Fake client cleaner could not be instantiated.")
	}
	defer cleanup()

	ownersServer := &OwnersServer{OwnersClient: client}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		req, err := http.NewRequest("POST", "/", bytes.NewBuffer(test.body))
		if err != nil {
			t.Fatalf("POST query to the EndPoint failed : %v.", err)
		}
		req.Header.Set("Content-Type", test.contentType)

		rr := httptest.NewRecorder()

		ownersServer.ServeHTTP(rr, req)
		if rr.Code != test.expectedCode {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, test.expectedCode)
		}
		if rr.Body.String() != test.expectedBody {
			t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), test.expectedBody)
		}
	}
}
