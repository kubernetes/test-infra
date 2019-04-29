package slack

import (
	"testing"
)

func TestEscapeText(t *testing.T) {
	testCases := []struct {
		in       string
		expected string
	}{
		{
			in:       "&",
			expected: "&amp;",
		},
		{
			in:       ">",
			expected: "&gt;",
		},
		{
			in:       "<",
			expected: "&lt;",
		},
		{
			in:       "<>&",
			expected: "&lt;&gt;&amp;",
		},
	}

	for _, tc := range testCases {
		if result := escapeText(tc.in); result != tc.expected {
			t.Errorf("Expected result to be %q but was %q", result, tc.expected)
		}
	}
}
