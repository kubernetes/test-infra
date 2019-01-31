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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	pjclientset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	runOnce                bool
	configPath             string
	jobConfigPath          string
	buildCluster           string
	buildClusterKubeconfig string
}

func gatherOptions() options {
	o := options{}
	flag.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.buildCluster, "build-cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
	flag.StringVar(&o.buildClusterKubeconfig, "kubeconfig", "", "Path to kubeconfig with build cluster credentials. If empty, defaults to in-cluster config.")
	flag.Parse()
	return o
}
func main() {
	o := gatherOptions()
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "sinker"}),
	)

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	clusterConfigs, defaultContext, err := kube.LoadClusterConfigs(o.buildClusterKubeconfig, o.buildCluster)
	defaultConfig := clusterConfigs[defaultContext]

	pjclient, err := pjclientset.NewForConfig(&defaultConfig)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating ProwJob client.")
	}

	var podClients []corev1.PodInterface
	for context, clusterConfig := range clusterConfigs {
		clusterClient, err := kubernetes.NewForConfig(&clusterConfig)
		if err != nil {
			logrus.WithError(err).Fatalf("Error creating Kubernetes client for context %q.", context)
		}
		podClients = append(podClients, clusterClient.CoreV1().Pods(configAgent.Config().PodNamespace))
	}

	c := controller{
		logger:        logrus.NewEntry(logrus.StandardLogger()),
		prowJobClient: pjclient.ProwV1().ProwJobs(cfg().ProwJobNamespace),
		podClients:    podClients,
		config:        cfg,
	}

	// Clean now and regularly from now on.
	for {
		start := time.Now()
		c.clean()
		logrus.Infof("Sync time: %v", time.Since(start))
		if o.runOnce {
			break
		}
		time.Sleep(cfg().Sinker.ResyncPeriod)
	}
}

type controller struct {
	logger        *logrus.Entry
	prowJobClient prowv1.ProwJobInterface
	podClients    []corev1.PodInterface
	config        config.Getter
}

func (c *controller) clean() {
	// Clean up old prow jobs first.
	prowJobs, err := c.prowJobClient.List(metav1.ListOptions{})
	if err != nil {
		c.logger.WithError(err).Error("Error listing prow jobs.")
		return
	}

	// Only delete pod if its prowjob is marked as finished
	isFinished := make(map[string]bool)

	maxProwJobAge := c.config().Sinker.MaxProwJobAge
	for _, prowJob := range prowJobs.Items {
		// Handle periodics separately.
		if prowJob.Spec.Type == prowapi.PeriodicJob {
			continue
		}
		if !prowJob.Complete() {
			continue
		}
		isFinished[prowJob.ObjectMeta.Name] = true
		if time.Since(prowJob.Status.StartTime.Time) <= maxProwJobAge {
			continue
		}
		if err := c.prowJobClient.Delete(prowJob.ObjectMeta.Name, &metav1.DeleteOptions{}); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
		}
	}

	// Keep track of what periodic jobs are in the config so we will
	// not clean up their last prowjob.
	isActivePeriodic := make(map[string]bool)
	for _, p := range c.config().Periodics {
		isActivePeriodic[p.Name] = true
	}

	// Get the jobs that we need to retain so horologium can continue working
	// as intended.
	latestPeriodics := pjutil.GetLatestProwJobs(prowJobs.Items, prowapi.PeriodicJob)
	for _, prowJob := range prowJobs.Items {
		if prowJob.Spec.Type != prowapi.PeriodicJob {
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
		if err := c.prowJobClient.Delete(prowJob.ObjectMeta.Name, &metav1.DeleteOptions{}); err == nil {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).Info("Deleted prowjob.")
		} else {
			c.logger.WithFields(pjutil.ProwJobFields(&prowJob)).WithError(err).Error("Error deleting prowjob.")
		}
	}

	// Now clean up old pods.
	selector := fmt.Sprintf("%s = %s", kube.CreatedByProw, "true")
	for _, client := range c.podClients {
		pods, err := client.List(metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			c.logger.WithError(err).Error("Error listing pods.")
			return
		}
		maxPodAge := c.config().Sinker.MaxPodAge
		for _, pod := range pods.Items {
			if _, ok := isFinished[pod.ObjectMeta.Name]; !ok {
				// prowjob is not marked as completed yet
				// deleting the pod now will result in plank creating a brand new pod
				continue
			}
			if !pod.Status.StartTime.IsZero() && time.Since(pod.Status.StartTime.Time) > maxPodAge {
				// Delete old completed pods. Don't quit if we fail to delete one.
				if err := client.Delete(pod.ObjectMeta.Name, &metav1.DeleteOptions{}); err == nil {
					c.logger.WithField("pod", pod.ObjectMeta.Name).Info("Deleted old completed pod.")
				} else {
					c.logger.WithField("pod", pod.ObjectMeta.Name).WithError(err).Error("Error deleting pod.")
				}
			}
		}
	}
}
