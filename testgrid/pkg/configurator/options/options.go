/*
Copyright 2021 The Kubernetes Authors.

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
	"errors"
	"flag"
	"strings"

	"sigs.k8s.io/prow/prow/flagutil"
	configflagutil "sigs.k8s.io/prow/prow/flagutil/config"

	"github.com/sirupsen/logrus"
)

type MultiString []string

func (m MultiString) String() string {
	return strings.Join(m, ",")
}

func (m *MultiString) Set(v string) error {
	*m = strings.Split(v, ",")
	return nil
}

type Options struct {
	Creds              string
	Inputs             MultiString
	Oneshot            bool
	Output             flagutil.Strings
	PrintText          bool
	ValidateConfigFile bool
	WorldReadable      bool
	WriteYAML          bool
	ProwConfig         configflagutil.ConfigOptions
	DefaultYAML        string
	UpdateDescription  bool
	ProwJobURLPrefix   string
	StrictUnmarshal    bool
}

func (o *Options) GatherOptions(fs *flag.FlagSet, args []string) error {
	fs.StringVar(&o.Creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	fs.BoolVar(&o.Oneshot, "oneshot", false, "Write proto once and exit instead of monitoring --yaml files for changes")
	fs.Var(&o.Output, "output", "write proto to gs://bucket/obj or /local/path")
	fs.BoolVar(&o.PrintText, "print-text", false, "print generated info in text format to stdout")
	fs.BoolVar(&o.ValidateConfigFile, "validate-config-file", false, "validate that the given config files are syntactically correct and exit (proto is not written anywhere)")
	fs.BoolVar(&o.WorldReadable, "world-readable", false, "when uploading the proto to GCS, makes it world readable. Has no effect on writing to the local filesystem.")
	fs.BoolVar(&o.WriteYAML, "output-yaml", false, "Output to TestGrid YAML instead of config proto")
	fs.Var(&o.Inputs, "yaml", "comma-separated list of input YAML files or directories")
	o.ProwConfig.ConfigPathFlagName = "prow-config"
	o.ProwConfig.JobConfigPathFlagName = "prow-job-config"
	o.ProwConfig.AddFlags(fs)
	fs.StringVar(&o.DefaultYAML, "default", "", "path to default settings; required for proto outputs")
	fs.BoolVar(&o.UpdateDescription, "update-description", false, "add prowjob info to description even if non-empty")
	fs.StringVar(&o.ProwJobURLPrefix, "prowjob-url-prefix", "", "for prowjob_config_url in descriptions: {prowjob-url-prefix}/{prowjob.sourcepath}")
	fs.BoolVar(&o.StrictUnmarshal, "strict-unmarshal", false, "whether or not we want to be strict when unmarshalling configs")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(o.Inputs) == 0 || o.Inputs[0] == "" {
		return errors.New("--yaml must include at least one file")
	}

	if !o.PrintText && !o.ValidateConfigFile && len(o.Output.Strings()) == 0 {
		return errors.New("--print-text, --validate-config-file, or --output required")
	}
	if o.ValidateConfigFile && len(o.Output.Strings()) > 0 {
		return errors.New("--validate-config-file doesn't write the proto anywhere")
	}
	if err := o.ProwConfig.ValidateConfigOptional(); err != nil {
		return err
	}
	if o.DefaultYAML == "" && !o.WriteYAML {
		logrus.Warnf("--default not explicitly specified; assuming %s", o.Inputs[0])
		o.DefaultYAML = o.Inputs[0]
	}
	return nil
}
