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

package options

import (
	"flag"
	"fmt"
	"os"
)

// OptionLoader allows loading options from either the environment or flags.
type OptionLoader interface {
	ConfigVar() string
	LoadConfig(config string) error
	AddFlags(flags *flag.FlagSet)
	Complete(args []string)
}

// Load loads the set of options, preferring to use
// JSON config from an env var, but falling back to
// command line flags if not possible.
func Load(loader OptionLoader) error {
	if jsonConfig, provided := os.LookupEnv(loader.ConfigVar()); provided {
		if err := loader.LoadConfig(jsonConfig); err != nil {
			return fmt.Errorf("could not load config from JSON var %s: %v", loader.ConfigVar(), err)
		}
		return nil
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loader.AddFlags(fs)
	fs.Parse(os.Args[1:])
	loader.Complete(fs.Args())

	return nil
}
