/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type kubeClient interface {
	ListPods(selector string) ([]kube.Pod, error)
	DeletePod(name string) error

	ListProwJobs(selector string) ([]kube.ProwJob, error)
	DeleteProwJob(name string) error
}

type configAgent interface {
	Config() *config.Config
}

var (
	runOnce      = flag.Bool("run-once", false, "If true, run only once then quit.")
	configPath   = flag.String("config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	buildCluster = flag.String("build-cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "sinker"}),
	)

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath, ""); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Error("Error getting client.")
		return
	}

	var pkcs map[string]*kube.Client
	if *buildCluster == "" {
		pkcs = map[string]*kube.Client{
			kube.DefaultClusterAlias: kc.Namespace(configAgent.Config().PodNamespace),
		}
	} else {
		pkcs, err = kube.ClientMapFromFile(*buildCluster, configAgent.Config().PodNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting kube client(s).")
		}
	}

	kubeClients := map[string]kubeClient{}
	for alias, client := range pkcs {
		kubeClients[alias] = kubeClient(client)
	}
	c := controller{
		logger:      logrus.NewEntry(logrus.StandardLogger()),
		kc:          kc,
		pkcs:        kubeClients,
		configAgent: configAgent,
	}

	// Clean now and regularly from now on.
	for {
		start := time.Now()
		c.clean()
		logrus.Infof("Sync time: %v", time.Since(start))
		if *runOnce {
			break
		}
		time.Sleep(configAgent.Config().Sinker.ResyncPeriod)
	}
}

type controller struct {
	logger      *logrus.Entry
	kc          kubeClient
	pkcs        map[string]kubeClient
	configAgent configAgent
}

func (c *controller) clean() {
	// Clean up old prow jobs first.
	prowJobs, err := c.kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		c.logger.WithError(err).Error("Error listing prow jobs.")
		return
	}

	// Only delete pod if its prowjob is marked as finished
	isFinished := make(map[string]bool)

	maxProwJobAge := c.configAgent.Config().Sinker.MaxProwJobAge
	for _, prowJob := range prowJobs {
		// Handle periodics separately.
		if prowJob.Spec.Type == kube.PeriodicJob {
			continue
		}
		if !prowJob.Complete() {
			continue
		}
		isFinished[prowJob.ObjectMeta.Name] = true
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if err := c.kc.DeleteProwJob(prowJob.ObjectMeta.Name); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
		}
	}

	// Keep track of what periodic jobs are in the config so we will
	// not clean up their last prowjob.
	isActivePeriodic := make(map[string]bool)
	for _, p := range c.configAgent.Config().Periodics {
		isActivePeriodic[p.Name] = true
	}

	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestProwJobs(prowJobs, kube.PeriodicJob)
	for _, prowJob := range prowJobs {
		if prowJob.Spec.Type != kube.PeriodicJob {
			continue
		}

		latestPJ := latestPeriodics[prowJob.Spec.Job]
		if isActivePeriodic[prowJob.Spec.Job] && prowJob.ObjectMeta.Name == latestPJ.ObjectMeta.Name {
			// Ignore deleting this one.
			continue
		}
		if !prowJob.Complete() {
			continue
		}
		isFinished[prowJob.ObjectMeta.Name] = true
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if err := c.kc.DeleteProwJob(prowJob.ObjectMeta.Name); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
		}
	}

	// Now clean up old pods.
	selector := fmt.Sprintf("%s = %s", kube.CreatedByProw, "true")
	for _, client := range c.pkcs {
		pods, err := client.ListPods(selector)
		if err != nil {
			c.logger.WithError(err).Error("Error listing pods.")
			return
		}
		maxPodAge := c.configAgent.Config().Sinker.MaxPodAge
		for _, pod := range pods {
			if _, ok := isFinished[pod.ObjectMeta.Name]; !ok {
				// prowjob is not marked as completed yet
				// deleting the pod now will result in plank creating a brand new pod
				continue
			}
			if !pod.Status.StartTime.IsZero() && time.Since(pod.Status.StartTime.Time) > maxPodAge {
				// Delete old completed pods. Don't quit if we fail to delete one.
				if err := client.DeletePod(pod.ObjectMeta.Name); err == nil {
					c.logger.WithField("pod", pod.ObjectMeta.Name).Info("Deleted old completed pod.")
				} else {
					c.logger.WithField("pod", pod.ObjectMeta.Name).WithError(err).Error("Error deleting pod.")
				}
			}
		}
	}
}
