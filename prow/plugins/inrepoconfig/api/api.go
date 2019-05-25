package api

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/clonerefs"
	"k8s.io/test-infra/prow/config"
)

const (
	// PluginName is the name of this plugin
	PluginName = "inrepoconfig"

	// CotnextName is the name of the context the inrepoconfig plugin creates at GitHub
	ContextName = "prow-config-parsing"

	// ConfigFileName is the name of the configfile the inrepoconfig plugin uses to read its
	// config from
	ConfigFileName = "prow.yaml"
)

// JobConfig is the format used to parse the prow.yaml
type JobConfig struct {
	Presubmits []config.Presubmit `json:"presubmits,omitempty"`
}

// defaultJobConfig defaults the JobConfig. This must be called before accessing any data in it
func (jc *JobConfig) defaultJobConfig(c *config.ProwConfig) {
	for i := range jc.Presubmits {
		config.DefaultPresubmitFields(c, &jc.Presubmits[i])
		jc.Presubmits[i].DecorationConfig = jc.Presubmits[i].DecorationConfig.ApplyDefault(c.Plank.DefaultDecorationConfig)
	}
}

func NewJobConfig(log *logrus.Entry, refs []prowapi.Refs, c *config.ProwConfig) (*JobConfig, error) {
	if len(refs) < 1 {
		return nil, errors.New("need at least one ref")
	}

	tempDir, err := ioutil.TempDir("/tmp", fmt.Sprintf("%s-%s-%s", refs[0].Org, refs[0].Repo, refs[0].BaseSHA))
	if err != nil {
		return nil, fmt.Errorf("failed to create tempDir for cloning: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.WithError(err).Errorf("failed to clean up temp directory %s", tempDir)
		}
	}()

	// TODO: Allow using SSH for cloning
	cloneOpts := clonerefs.Options{
		GitUserEmail: "denna@protonmail.com",
		SrcRoot:      tempDir,
		Log:          "/dev/null",
		GitRefs:      refs,
	}
	if err := cloneOpts.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone: %v", err)
	}

	configFile := fmt.Sprintf("%s/src/github.com/%s/%s/%s", tempDir, refs[0].Org, refs[0].Repo, ConfigFileName)
	if _, err := os.Stat(configFile); err != nil {
		if os.IsNotExist(err) {
			return &JobConfig{}, nil
		}
		return nil, fmt.Errorf("failed to check if %q exists: %v", ConfigFileName, err)
	}

	configRaw, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %v", ConfigFileName, err)
	}

	jc := &JobConfig{}
	if err := yaml.UnmarshalStrict(configRaw, jc); err != nil {
		return nil, fmt.Errorf("failed to parse %q: %v", ConfigFileName, err)
	}

	jc.defaultJobConfig(c)

	return jc, nil
}
