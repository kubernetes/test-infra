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

package updateconfig

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"unicode/utf8"

	"github.com/mattn/go-zglob"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName    = "config-updater"
	bootstrapMode = false
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	var configInfo map[string]string
	if len(enabledRepos) == 1 {
		msg := ""
		for configFileName, configMapSpec := range config.ConfigUpdater.Maps {
			msg = msg + fmt.Sprintf(
				"Files matching %s/%s are used to populate the %s ConfigMap in ",
				enabledRepos[0],
				configFileName,
				configMapSpec.Name,
			)
			if len(configMapSpec.AdditionalNamespaces) == 0 {
				msg = msg + fmt.Sprintf("the %s namespace.\n", configMapSpec.Namespace)
			} else {
				for _, nameSpace := range configMapSpec.AdditionalNamespaces {
					msg = msg + fmt.Sprintf("%s, ", nameSpace)
				}
				msg = msg + fmt.Sprintf("and %s namespaces.\n", configMapSpec.Namespace)
			}
		}
		configInfo = map[string]string{"": msg}
	}
	return &pluginhelp.PluginHelp{
			Description: "The config-updater plugin automatically redeploys configuration and plugin configuration files when they change. The plugin watches for pull request merges that modify either of the config files and updates the cluster's configmap resources in response.",
			Config:      configInfo,
		},
		nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.GitClient, pc.KubernetesClient.CoreV1(), pc.BuildClusterCoreV1Clients, pc.Config.ProwJobNamespace, pc.Logger, pre, pc.PluginConfig.ConfigUpdater, pc.Metrics.ConfigMapGauges)
}

// FileGetter knows how to get the contents of a file by name
type FileGetter interface {
	GetFile(filename string) ([]byte, error)
}

type OSFileGetter struct {
	Root string
}

func (g *OSFileGetter) GetFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(g.Root, filename))
}

// Update updates the configmap with the data from the identified files.
// Existing configmap keys that are not included in the updates are left alone
// unless bootstrap is true in which case they are deleted.
func Update(fg FileGetter, kc corev1.ConfigMapInterface, name, namespace string, updates []ConfigMapUpdate, bootstrap bool, metrics *prometheus.GaugeVec, logger *logrus.Entry) error {
	cm, getErr := kc.Get(name, metav1.GetOptions{})
	isNotFound := errors.IsNotFound(getErr)
	if getErr != nil && !isNotFound {
		return fmt.Errorf("failed to fetch current state of configmap: %v", getErr)
	}

	if cm == nil || isNotFound {
		cm = &coreapi.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}
	if cm.Data == nil || bootstrap {
		cm.Data = map[string]string{}
	}
	if cm.BinaryData == nil || bootstrap {
		cm.BinaryData = map[string][]byte{}
	}

	for _, upd := range updates {
		if upd.Filename == "" {
			logger.WithField("key", upd.Key).Debug("Deleting key.")
			delete(cm.Data, upd.Key)
			delete(cm.BinaryData, upd.Key)
			continue
		}

		content, err := fg.GetFile(upd.Filename)
		if err != nil {
			return fmt.Errorf("get file err: %v", err)
		}
		logger.WithFields(logrus.Fields{"key": upd.Key, "filename": upd.Filename}).Debug("Populating key.")
		value := content
		if upd.GZIP {
			buff := bytes.NewBuffer([]byte{})
			// TODO: this error is wildly unlikely for anything that
			// would actually fit in a configmap, we could just as well return
			// the error instead of falling back to the raw content
			z := gzip.NewWriter(buff)
			if _, err := z.Write(content); err != nil {
				logger.WithError(err).Error("failed to gzip content, falling back to raw")
			} else {
				if err := z.Close(); err != nil {
					logger.WithError(err).Error("failed to flush gzipped content (!?), falling back to raw")
				} else {
					value = buff.Bytes()
				}
			}
		}
		if utf8.ValidString(string(value)) {
			delete(cm.BinaryData, upd.Key)
			cm.Data[upd.Key] = string(value)
		} else {
			delete(cm.Data, upd.Key)
			cm.BinaryData[upd.Key] = value
		}
	}

	var updateErr error
	var verb string
	if getErr != nil && isNotFound {
		verb = "create"
		_, updateErr = kc.Create(cm)
	} else {
		verb = "update"
		_, updateErr = kc.Update(cm)
	}
	if updateErr != nil {
		return fmt.Errorf("%s config map err: %v", verb, updateErr)
	}
	if metrics != nil {
		var size float64
		for _, data := range cm.Data {
			size += float64(len(data))
		}
		// in a strict sense this can race to update the value with other goroutines
		// handling other events, but as events are serialized due to the fact that
		// merges are serial in repositories, this is effectively not an issue here
		metrics.WithLabelValues(cm.Name, cm.Namespace).Set(size)
	}
	return nil
}

// ConfigMapUpdate is populated with information about a config map that should
// be updated.
type ConfigMapUpdate struct {
	Key, Filename string
	GZIP          bool
}

// FilterChanges determines which of the changes are relevant for config updating, returning mapping of
// config map to key to filename to update that key from.
func FilterChanges(cfg plugins.ConfigUpdater, changes []github.PullRequestChange, defaultNamespace string, log *logrus.Entry) map[plugins.ConfigMapID][]ConfigMapUpdate {
	toUpdate := map[plugins.ConfigMapID][]ConfigMapUpdate{}
	for _, change := range changes {
		var cm plugins.ConfigMapSpec
		found := false

		for key, configMap := range cfg.Maps {
			var matchErr error
			found, matchErr = zglob.Match(key, change.Filename)
			if matchErr != nil {
				// Should not happen, log matchErr and continue
				log.WithError(matchErr).Info("key matching error")
				continue
			}

			if found {
				cm = configMap
				break
			}
		}

		if !found {
			continue // This file does not define a configmap
		}

		// Yes, update the configmap with the contents of this file
		for cluster, namespaces := range cm.Clusters {
			for _, ns := range namespaces {
				id := plugins.ConfigMapID{Name: cm.Name, Namespace: ns, Cluster: cluster}
				key := cm.Key
				if key == "" {
					key = path.Base(change.Filename)
					// if the key changed, we need to remove the old key
					if change.Status == github.PullRequestFileRenamed {
						oldKey := path.Base(change.PreviousFilename)
						// not setting the filename field will cause the key to be
						// deleted
						toUpdate[id] = append(toUpdate[id], ConfigMapUpdate{Key: oldKey})
					}
				}
				if change.Status == github.PullRequestFileRemoved {
					toUpdate[id] = append(toUpdate[id], ConfigMapUpdate{Key: key})
				} else {
					gzip := cfg.GZIP
					if cm.GZIP != nil {
						gzip = *cm.GZIP
					}
					toUpdate[id] = append(toUpdate[id], ConfigMapUpdate{Key: key, Filename: change.Filename, GZIP: gzip})
				}
			}
		}
	}
	return handleDefaultNamespace(toUpdate, defaultNamespace)
}

// handleDefaultNamespace ensures plugins.ConfigMapID.Namespace is not empty string
func handleDefaultNamespace(toUpdate map[plugins.ConfigMapID][]ConfigMapUpdate, defaultNamespace string) map[plugins.ConfigMapID][]ConfigMapUpdate {
	for cm, data := range toUpdate {
		if cm.Namespace == "" {
			key := plugins.ConfigMapID{Name: cm.Name, Namespace: defaultNamespace, Cluster: cm.Cluster}
			toUpdate[key] = append(toUpdate[key], data...)
			delete(toUpdate, cm)
		}
	}
	return toUpdate
}

func handle(gc githubClient, gitClient git.ClientFactory, kc corev1.ConfigMapsGetter, buildClusterCoreV1Clients map[string]corev1.CoreV1Interface, defaultNamespace string, log *logrus.Entry, pre github.PullRequestEvent, config plugins.ConfigUpdater, metrics *prometheus.GaugeVec) error {
	// Only consider newly merged PRs
	if pre.Action != github.PullRequestActionClosed {
		return nil
	}

	if len(config.Maps) == 0 { // Nothing to update
		return nil
	}

	pr := pre.PullRequest

	if !pr.Merged || pr.MergeSHA == nil || pr.Base.Repo.DefaultBranch != pr.Base.Ref {
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name

	// Which files changed in this PR?
	changes, err := gc.GetPullRequestChanges(org, repo, pr.Number)
	if err != nil {
		return err
	}

	message := func(name, cluster, namespace string, updates []ConfigMapUpdate, indent string) string {
		identifier := fmt.Sprintf("`%s` configmap", name)
		if namespace != "" {
			identifier = fmt.Sprintf("%s in namespace `%s`", identifier, namespace)
		}
		if cluster != "" {
			identifier = fmt.Sprintf("%s at cluster `%s`", identifier, cluster)
		}
		msg := fmt.Sprintf("%s using the following files:", identifier)
		for _, u := range updates {
			msg = fmt.Sprintf("%s\n%s- key `%s` using file `%s`", msg, indent, u.Key, u.Filename)
		}
		return msg
	}

	// Are any of the changes files ones that define a configmap we want to update?
	toUpdate := FilterChanges(config, changes, defaultNamespace, log)
	log.WithFields(logrus.Fields{
		"configmaps_to_update": len(toUpdate),
		"changes":              len(changes),
	}).Debug("Identified configmaps to update")

	var updated []string
	indent := " " // one space
	if len(toUpdate) > 1 {
		indent = "   " // three spaces for sub bullets
	}

	gitRepo, err := gitClient.ClientFor(org, repo)
	if err != nil {
		return err
	}
	defer func() {
		if err := gitRepo.Clean(); err != nil {
			log.WithError(err).Error("Could not clean up git repo cache.")
		}
	}()
	if err := gitRepo.Checkout(*pr.MergeSHA); err != nil {
		return err
	}

	var errs []error
	for cm, data := range toUpdate {
		logger := log.WithFields(logrus.Fields{"configmap": map[string]string{"name": cm.Name, "namespace": cm.Namespace, "cluster": cm.Cluster}})
		configMapClient, err := GetConfigMapClient(kc, cm.Namespace, buildClusterCoreV1Clients, cm.Cluster)
		if err != nil {
			log.WithError(err).Errorf("Failed to find configMap client")
			errs = append(errs, err)
			continue
		}
		if err := Update(&OSFileGetter{Root: gitRepo.Directory()}, configMapClient, cm.Name, cm.Namespace, data, bootstrapMode, metrics, logger); err != nil {
			errs = append(errs, err)
			continue
		}
		updated = append(updated, message(cm.Name, cm.Cluster, cm.Namespace, data, indent))
	}

	var msg string
	switch n := len(updated); n {
	case 0:
		return utilerrors.NewAggregate(errs)
	case 1:
		msg = fmt.Sprintf("Updated the %s", updated[0])
	default:
		msg = fmt.Sprintf("Updated the following %d configmaps:\n", n)
		for _, updateMsg := range updated {
			msg += fmt.Sprintf(" * %s\n", updateMsg) // one space indent
		}
	}

	if err := gc.CreateComment(org, repo, pr.Number, plugins.FormatResponseRaw(pr.Body, pr.HTMLURL, pr.User.Login, msg)); err != nil {
		errs = append(errs, fmt.Errorf("comment err: %v", err))
	}
	return utilerrors.NewAggregate(errs)
}

// GetConfigMapClient returns a configMap interface according to the given cluster and namespace
func GetConfigMapClient(kc corev1.ConfigMapsGetter, namespace string, buildClusterCoreV1Clients map[string]corev1.CoreV1Interface, cluster string) (corev1.ConfigMapInterface, error) {
	configMapClient := kc.ConfigMaps(namespace)
	if cluster != kube.DefaultClusterAlias {
		if client, ok := buildClusterCoreV1Clients[cluster]; ok {
			configMapClient = client.ConfigMaps(namespace)
		} else {
			return nil, fmt.Errorf("no k8s client is found for build cluster: '%s'", cluster)
		}
	}
	return configMapClient, nil
}
