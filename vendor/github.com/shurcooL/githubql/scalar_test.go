package githubql_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/shurcooL/githubql"
)

func TestURI_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   githubql.URI
		want string
	}{
		{
			in:   githubql.URI{URL: &url.URL{Scheme: "https", Host: "example.org", Path: "/foo/bar"}},
			want: `"https://example.org/foo/bar"`,
		},
	}
	for _, tc := range tests {
		got, err := json.Marshal(tc.in)
		if err != nil {
			t.Fatalf("%s: got error: %v", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s: got: %q, want: %q", tc.name, string(got), tc.want)
		}
	}
}

func TestURI_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      githubql.URI
		wantError error
	}{
		{
			in:   `"https://example.org/foo/bar"`,
			want: githubql.URI{URL: &url.URL{Scheme: "https", Host: "example.org", Path: "/foo/bar"}},
		},
		{
			name: "null",
			in:   `null`,
			want: githubql.URI{},
		},
		{
			name:      "error JSON unmarshaling into string",
			in:        `86`,
			wantError: errors.New("json: cannot unmarshal number into Go value of type string"),
		},
	}
	for _, tc := range tests {
		var got githubql.URI
		err := json.Unmarshal([]byte(tc.in), &got)
		if got, want := err, tc.wantError; !equalError(got, want) {
			t.Fatalf("%s: got error: %v, want: %v", tc.name, got, want)
		}
		if tc.wantError != nil {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got: %v, want: %v", tc.name, got, tc.want)
		}
	}
}

// equalError reports whether errors a and b are considered equal.
// They're equal if both are nil, or both are not nil and a.Error() == b.Error().
func equalError(a, b error) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Error() == b.Error()
}

func TestNewScalars(t *testing.T) {
	if got := githubql.NewBoolean(false); got == nil {
		t.Error("NewBoolean returned nil")
	}
	if got := githubql.NewDateTime(githubql.DateTime{}); got == nil {
		t.Error("NewDateTime returned nil")
	}
	if got := githubql.NewFloat(0.0); got == nil {
		t.Error("NewFloat returned nil")
	}
	if got := githubql.NewGitObjectID(""); got == nil {
		t.Error("NewGitObjectID returned nil")
	}
	if got := githubql.NewGitTimestamp(githubql.GitTimestamp{}); got == nil {
		t.Error("NewGitTimestamp returned nil")
	}
	if got := githubql.NewHTML(""); got == nil {
		t.Error("NewHTML returned nil")
	}
	// ID with underlying type string.
	if got := githubql.NewID(""); got == nil {
		t.Error("NewID returned nil")
	}
	// ID with underlying type int.
	if got := githubql.NewID(0); got == nil {
		t.Error("NewID returned nil")
	}
	if got := githubql.NewInt(0); got == nil {
		t.Error("NewInt returned nil")
	}
	if got := githubql.NewString(""); got == nil {
		t.Error("NewString returned nil")
	}
	if got := githubql.NewURI(githubql.URI{}); got == nil {
		t.Error("NewURI returned nil")
	}
	if got := githubql.NewX509Certificate(githubql.X509Certificate{}); got == nil {
		t.Error("NewX509Certificate returned nil")
	}
}
