package secret

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadSingleSecret(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []byte
		wantErr bool
	}{
		{"valid token",  `121f3cb3e7f70feeb35f9204f5a988d7292c7ba1`, []byte("121f3cb3e7f70feeb35f9204f5a988d7292c7ba1"), false},
		{"valid token with surrounding whitespace",  ` 121f3cb3e7f70feeb35f9204f5a988d7292c7ba1
`, []byte("121f3cb3e7f70feeb35f9204f5a988d7292c7ba1"), false},
		{"token containing linesbreak", `121f3cb3e7f70feeb35f
9204f5a988d7292c7ba1`, nil, true},
	}

	// Creating a temporary directory.
	secretDir, err := ioutil.TempDir("", "secretDir")
	if err != nil {
		t.Fatalf("fail to create a temporary directory: %v", err)
	}
	defer os.RemoveAll(secretDir)
 	tempSecret := filepath.Join(secretDir, "tempSecret")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ioutil.WriteFile(tempSecret, []byte(tt.content), 0666); err != nil {
				t.Fatalf("fail to write secret: %v", err)
			}
			got, err := LoadSingleSecret(tempSecret)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadSingleSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadSingleSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}