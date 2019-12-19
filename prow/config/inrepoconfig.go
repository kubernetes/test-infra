package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/git/v2"
	"sigs.k8s.io/yaml"
)

const (
	inRepoConfigFileName = ".prow.yaml"
)

// ProwYAML represents the content of a .prow.yaml file
// used to version Presubmits inside the tested repo.
type ProwYAML struct {
	Presubmits []Presubmit `json:"presubmits"`
}

// ProwYAMLGetter is used to retrieve a ProwYAML. Tests should provide
// their own implementation and set that on the Config.
type ProwYAMLGetter func(c *Config, gc git.ClientFactory, identifier, baseSHA string, headSHAs ...string) (*ProwYAML, error)

// Verify defaultProwYAMLGetter is a ProwYAMLGetter
var _ ProwYAMLGetter = defaultProwYAMLGetter

func defaultProwYAMLGetter(
	c *Config,
	gc git.ClientFactory,
	identifier string,
	baseSHA string,
	headSHAs ...string) (*ProwYAML, error) {

	log := logrus.WithField("repo", identifier)
	log.Debugf("Attempting to get %q.", inRepoConfigFileName)

	if gc == nil {
		log.Error("defaultProwYAMLGetter was called with a nil git client")
		return nil, errors.New("gitClient is nil")
	}

	identifierSlashSplit := strings.Split(identifier, "/")
	if len(identifierSlashSplit) != 2 {
		return nil, fmt.Errorf("didn't get two but %d results when splitting repo identifier %q", len(identifierSlashSplit), identifier)
	}
	organization, repository := identifierSlashSplit[0], identifierSlashSplit[1]
	repo, err := gc.ClientFor(organization, repository)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo for %q: %v", identifier, err)
	}
	defer func() {
		if err := repo.Clean(); err != nil {
			log.WithError(err).Error("Failed to clean up repo.")
		}
	}()

	if err := repo.Config("user.name", "prow"); err != nil {
		return nil, err
	}
	if err := repo.Config("user.email", "prow@localhost"); err != nil {
		return nil, err
	}
	if err := repo.Config("commit.gpgsign", "false"); err != nil {
		return nil, err
	}

	mergeMethod := c.Tide.MergeMethod(organization, repository)
	log.Debugf("Using merge strategy %q.", mergeMethod)
	if err := repo.MergeAndCheckout(baseSHA, string(mergeMethod), headSHAs...); err != nil {
		return nil, fmt.Errorf("failed to merge: %v", err)
	}

	prowYAMLFilePath := path.Join(repo.Directory(), inRepoConfigFileName)
	if _, err := os.Stat(prowYAMLFilePath); err != nil {
		if os.IsNotExist(err) {
			log.Debugf("File %q does not exist.", inRepoConfigFileName)
			return &ProwYAML{}, nil
		}
		return nil, fmt.Errorf("failed to check if file %q exists: %v", inRepoConfigFileName, err)
	}

	bytes, err := ioutil.ReadFile(prowYAMLFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %v", inRepoConfigFileName, err)
	}

	prowYAML := &ProwYAML{}
	if err := yaml.Unmarshal(bytes, prowYAML); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %q: %v", inRepoConfigFileName, err)
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	log.Debugf("Successfully got %d presubmits from %q.", len(prowYAML.Presubmits), inRepoConfigFileName)
	return prowYAML, nil
}

func DefaultAndValidateProwYAML(c *Config, p *ProwYAML, identifier string) error {
	if err := defaultPresubmits(p.Presubmits, c, identifier); err != nil {
		return err
	}
	if err := validatePresubmits(append(p.Presubmits, c.PresubmitsStatic[identifier]...), c.PodNamespace); err != nil {
		return err
	}

	var errs []error
	for _, ps := range p.Presubmits {
		if ps.Branches != nil || ps.SkipBranches != nil {
			errs = append(errs, fmt.Errorf("job %q contains branchconfig. This is not allowed for jobs in %q", ps.Name, inRepoConfigFileName))
		}
		if !c.InRepoConfigAllowsCluster(ps.Cluster, identifier) {
			errs = append(errs, fmt.Errorf("cluster %q is not allowed for repository %q", ps.Cluster, identifier))
		}
	}

	return utilerrors.NewAggregate(errs)
}
