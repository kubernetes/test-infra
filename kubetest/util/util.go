/*
Copyright 2017 The Kubernetes Authors.

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

package util

import (
	"encoding/json"
	"fmt"
	"go/build"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// K8s returns $GOPATH/src/k8s.io/...
func K8s(topdir string, parts ...string) string {
	gopathList := filepath.SplitList(build.Default.GOPATH)
	found := false
	var kubedir string
	for _, gopath := range gopathList {
		kubedir = filepath.Join(gopath, "src", "k8s.io", topdir)
		if _, err := os.Stat(kubedir); !os.IsNotExist(err) {
			found = true
			break
		}
	}
	if !found {
		// Default to the first item in GOPATH list.
		kubedir = filepath.Join(gopathList[0], "src", "k8s.io", topdir)
		log.Printf(
			"Warning: Couldn't find directory src/k8s.io/%s under any of GOPATH %s, defaulting to %s",
			topdir, build.Default.GOPATH, kubedir)
	}
	p := []string{kubedir}
	p = append(p, parts...)
	return filepath.Join(p...)
}

// AppendError does append(errs, err) if err != nil
func AppendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

// Home returns $HOME/part/part/part
func Home(parts ...string) string {
	p := []string{os.Getenv("HOME")}
	for _, a := range parts {
		p = append(p, a)
	}
	return filepath.Join(p...)
}

// InsertPath does export PATH=path:$PATH
func InsertPath(path string) error {
	return os.Setenv("PATH", fmt.Sprintf("%v:%v", path, os.Getenv("PATH")))
}

// OptionalAbsPath returns an absolute path if the provided path wasn't empty, and otherwise
// returns an empty string.
func OptionalAbsPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	return filepath.Abs(path)
}

// JoinURL converts input (gs://foo, "bar") to gs://foo/bar
func JoinURL(urlPath, path string) (string, error) {
	u, err := url.Parse(urlPath)
	if err != nil {
		return "", err
	}
	u.Path = filepath.Join(u.Path, path)
	return u.String(), nil
}

// Pushd will Chdir() to dir and return a function to cd back to Getwd()
func Pushd(dir string) (func() error, error) {
	old, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to os.Getwd(): %v", err)
	}
	if err = os.Chdir(dir); err != nil {
		return nil, err
	}
	return func() error {
		return os.Chdir(old)
	}, nil
}

// PushEnv pushes env=value and return a function that resets env
func PushEnv(env, value string) (func() error, error) {
	prev, present := os.LookupEnv(env)
	if err := os.Setenv(env, value); err != nil {
		return nil, fmt.Errorf("could not set %s: %v", env, err)
	}
	return func() error {
		if present {
			return os.Setenv(env, prev)
		}
		return os.Unsetenv(env)
	}, nil
}

// MigratedOption is an option that was an ENV that is now a --flag
type MigratedOption struct {
	Env      string  // env associated with --flag
	Option   *string // Value of --flag
	Name     string  // --flag name
	SkipPush bool    // Push option to env if false
}

// MigrateOptions reads value from ENV if --flag unset, optionally pushing to ENV
func MigrateOptions(m []MigratedOption) error {
	for _, s := range m {
		if *s.Option == "" {
			// Jobs may not be using --foo instead of FOO just yet, so ease the transition
			// TODO(fejta): require --foo instead of FOO
			v := os.Getenv(s.Env) // expected Getenv
			if v != "" {
				// Tell people to use --foo=blah instead of FOO=blah
				log.Printf("Please use kubetest %s=%s (instead of deprecated %s=%s)", s.Name, v, s.Env, v)
				*s.Option = v
			}
		}
		if s.SkipPush {
			continue
		}
		// Script called by kubetest may expect these values to be set, so set them
		// TODO(fejta): refactor the scripts below kubetest to use explicit config
		if *s.Option == "" {
			continue
		}
		if err := os.Setenv(s.Env, *s.Option); err != nil {
			return fmt.Errorf("could not set %s=%s: %v", s.Env, *s.Option, err)
		}
	}
	return nil
}

// AppendField will append prefix to the flag value.
//
// For example, AppendField(fields, "--foo", "bar") if fields is empty or does
// not contain a "--foo" it will simply append a "--foo=bar" value.
// Otherwise if fields contains "--foo=current" it will replace this value with
// "--foo=current-bar
func AppendField(fields []string, flag, prefix string) []string {
	fields, cur, _ := ExtractField(fields, flag)
	if len(cur) == 0 {
		cur = prefix
	} else {
		cur += "-" + prefix
	}
	return append(fields, flag+"="+cur)
}

// SetFieldDefault sets the value of flag to val if flag is not present in fields.
//
// For example, SetFieldDefault(fields, "--foo", "bar") will append "--foo=bar" if
// fields is empty or does not include a "--foo" flag.
// It returns fields unchanged if "--foo" is present.
func SetFieldDefault(fields []string, flag, val string) []string {
	fields, cur, present := ExtractField(fields, flag)
	if !present {
		cur = val
	}
	return append(fields, flag+"="+cur)
}

// ExtractField input ("--a=this --b=that --c=other", "--b") returns [--a=this, --c=other"], "that", true
//
// In other words, it will remove "--b" from fields and return the previous value of "--b" if it was set.
func ExtractField(fields []string, target string) ([]string, string, bool) {
	f := []string{}
	prefix := target + "="
	consumeNext := false
	done := false
	r := ""
	for _, field := range fields {
		switch {
		case done:
			f = append(f, field)
		case consumeNext:
			r = field
			done = true
		case field == target:
			consumeNext = true
		case strings.HasPrefix(field, prefix):
			r = strings.SplitN(field, "=", 2)[1]
			done = true
		default:
			f = append(f, field)
		}
	}
	return f, r, done
}

// ExecError returns a string format of err including stderr if the
// err is an ExitError, useful for errors from e.g. exec.Cmd.Output().
func ExecError(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Sprintf("%v (output: %q)", err, string(ee.Stderr))
	}
	return err.Error()
}

// EnsureExecutable sets the executable file mode bits, for all users, to ensure that we can execute a file
func EnsureExecutable(p string) error {
	s, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("error doing stat on %q: %v", p, err)
	}
	if err := os.Chmod(p, s.Mode()|0111); err != nil {
		return fmt.Errorf("error doing chmod on %q: %v", p, err)
	}
	return nil
}

// JSONForDebug returns a json representation of the value, or a string representation of an error
// It is useful for an easy implementation of fmt.Stringer
func JSONForDebug(o interface{}) string {
	if o == nil {
		return "nil"
	}
	v, err := json.Marshal(o)
	if err != nil {
		return fmt.Sprintf("error[%v]", err)
	}
	return string(v)
}

// FlushMem will try to reduce the memory usage of the container it is running in
// run this after a build
func FlushMem() {
	log.Println("Flushing memory.")
	// it's ok if these fail
	// flush memory buffers
	err := exec.Command("sync").Run()
	if err != nil {
		log.Printf("flushMem error (sync): %v", err)
	}
	// clear page cache
	err = exec.Command("bash", "-c", "echo 1 > /proc/sys/vm/drop_caches").Run()
	if err != nil {
		log.Printf("flushMem error (page cache): %v", err)
	}
}
