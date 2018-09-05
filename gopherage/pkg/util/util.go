package util

import (
	"k8s.io/test-infra/gopherage/pkg/cov"
	"fmt"
	"os"
	"io"
	"golang.org/x/tools/cover"
)

func DumpProfile(destination string, profile []*cover.Profile) error {
	var output io.Writer
	if destination == "-" {
		output = os.Stdout
	} else {
		f, err := os.Create(destination)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", destination, err)
		}
		defer f.Close()
		output = f
	}
	err := cov.DumpProfile(profile, output)
	if err != nil {
		return fmt.Errorf("failed to dump profile: %v", err)
	}
	return nil
}
