package edit

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/bazelbuild/buildtools/build"
)

type defaultBuildifier struct{}

// buildify formats the build file f.
// Runs opts.Buildifier if it's non-empty, otherwise uses built-in formatter.
// opts.Buildifier is useful to force consistency with other tools that call Buildifier.
func (b *defaultBuildifier) Buildify(opts *Options, f *build.File) ([]byte, error) {
	if opts.Buildifier == "" {
		// Current AST may be not entirely correct, e.g. it may contain Ident which
		// value is a chunk of code, like "f(x)". The AST should be printed and
		// re-read to parse such expressions correctly.
		contents := build.Format(f)
		newF, err := build.ParseBuild(f.Path, []byte(contents))
		if err != nil {
			return nil, err
		}
		return build.Format(newF), nil
	}

	cmd := exec.Command(opts.Buildifier, "--type=build")
	data := build.Format(f)
	cmd.Stdin = bytes.NewBuffer(data)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(
        	os.Environ(),
	        // Custom environment variables
	)
	err := cmd.Run()
	if stderr.Len() > 0 {
		return nil, fmt.Errorf("%s", stderr.Bytes())
	}
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}
