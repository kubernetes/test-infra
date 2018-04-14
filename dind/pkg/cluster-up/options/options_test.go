package options

import (
	"flag"
	"reflect"
	"testing"
)

// TestCLIOptions tests that the options correctly parses from the CLI.
func TestCLIOptions(t *testing.T) {
	testCases := []struct {
		testName        string
		argv0           string
		args            []string
		expectedSuccess bool
		expectedOptions Options
	}{
		{
			"Expected sample happy path",
			"foo",
			[]string{"--side-load-image=false", "--proxy-addr=192.168.0.1", "--num-nodes=3"},
			true,
			Options{
				SideloadImage: false,
				DinDNodeImage: "k8s.gcr.io/dind-node-amd64",
				ProxyAddr:     "192.168.0.1",
				Version:       "",
				NumNodes:      3,
			},
		},
		{
			"Negative nodes should fail",
			"foo",
			[]string{"--num-nodes=0"},
			false,
			Options{},
		},
	}

	for _, tc := range testCases {
		set := flag.NewFlagSet(tc.argv0, flag.ContinueOnError)
		o, err := New(set, tc.args)

		if err == nil && !tc.expectedSuccess {
			t.Errorf("Test %q expected error, but got %v", tc.testName, o)
		}
		if err != nil && tc.expectedSuccess {
			t.Errorf("Test %q expected success, but got error %v", tc.testName, err)
		}
		if err != nil && !tc.expectedSuccess {
			continue
		}

		if !reflect.DeepEqual(*o, tc.expectedOptions) {
			t.Errorf("Test case %q expected %#v but got %#v", tc.testName, tc.expectedOptions, *o)
		}
	}
}
