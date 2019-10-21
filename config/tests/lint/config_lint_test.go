package tests

import (
	"os"
	"path/filepath"
	"testing"
)

var configPath = "../../../config/"

func Test_ForbidYmlExtension(t *testing.T) {
	err := filepath.Walk(configPath, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".yml" {
			t.Errorf("*.yml extension not allowed in this repository's configuration; use *.yaml instead (at %s)", path)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}
