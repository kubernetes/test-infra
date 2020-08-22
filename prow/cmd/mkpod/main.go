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

package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"k8s.io/test-infra/prow/pjutil"
	"os"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

type options struct {
	prowJobPath string
	buildID     string

	localMode bool
	outputDir string
}

func (o *options) Validate() error {
	if o.prowJobPath == "" {
		return errors.New("required flag --prow-job was unset")
	}

	if !o.localMode && o.outputDir != "" {
		return errors.New("out-dir may only be specified in --local mode")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.prowJobPath, "prow-job", "", "ProwJob to decorate, - for stdin.")
	flag.StringVar(&o.buildID, "build-id", "", "Build ID for the job run or 'snowflake' to generate one. Use 'snowflake' if tot is not used.")
	flag.BoolVar(&o.localMode, "local", false, "Configures pod utils for local mode which avoids uploading to GCS and the need for credentials. Instead, files are copied to a directory on the host. Hint: This works great with kind!")
	flag.StringVar(&o.outputDir, "out-dir", "", "Only allowed in --local mode. This is the directory to 'upload' to instead of GCS. If unspecified a temp dir is created.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	var rawJob []byte
	if o.prowJobPath == "-" {
		raw, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read ProwJob YAML from stdin.")
		}
		rawJob = raw
	} else {
		raw, err := ioutil.ReadFile(o.prowJobPath)
		if err != nil {
			logrus.WithError(err).Fatal("Could not open ProwJob YAML.")
		}
		rawJob = raw
	}

	var job prowapi.ProwJob
	if err := yaml.Unmarshal(rawJob, &job); err != nil {
		logrus.WithError(err).Fatal("Could not unmarshal ProwJob YAML.")
	}

	if o.buildID == "" && job.Status.BuildID != "" {
		o.buildID = job.Status.BuildID
	}

	if strings.ToLower(o.buildID) == "snowflake" {
		// No error possible since this won't use tot.
		o.buildID, _ = pjutil.GetBuildID(job.Spec.Job, "")
		logrus.WithField("build-id", o.buildID).Info("Generated build-id for job.")
	}

	if o.buildID == "" {
		logrus.Warning("No BuildID found in ProwJob status or given with --build-id, GCS interaction will be poor.")
	}

	var pod *v1.Pod
	var err error
	if o.localMode {
		outDir := o.outputDir
		if outDir == "" {
			prefix := strings.Join([]string{"prowjob-out", job.Spec.Job, o.buildID}, "-")
			logrus.Infof("Creating temp directory for job output in %q with prefix %q.", os.TempDir(), prefix)
			outDir, err = ioutil.TempDir("", prefix)
			if err != nil {
				logrus.WithError(err).Fatal("Could not create temp directory for job output.")
			}
		} else {
			outDir = path.Join(outDir, o.buildID)
		}
		logrus.WithField("out-dir", outDir).Info("Pod-utils configured for local mode. Instead of uploading to GCS, files will be copied to an output dir on the node.")

		pod, err = makeLocalPod(job, o.buildID, outDir)
		if err != nil {
			logrus.WithError(err).Fatal("Could not decorate PodSpec for local mode.")
		}
	} else {
		pod, err = decorate.ProwJobToPod(job, o.buildID)
		if err != nil {
			logrus.WithError(err).Fatal("Could not decorate PodSpec.")
		}
	}

	// We need to remove the created-by-prow label, otherwise sinker will promptly clean this
	// up as there is no associated prowjob
	newLabels := map[string]string{}
	for k, v := range pod.Labels {
		if k == kube.CreatedByProw {
			continue
		}
		newLabels[k] = v
	}
	pod.Labels = newLabels

	pod.GetObjectKind().SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("Pod"))
	podYAML, err := yaml.Marshal(pod)
	if err != nil {
		logrus.WithError(err).Fatal("Could not marshal Pod YAML.")
	}
	fmt.Println(string(podYAML))
}

func makeLocalPod(pj prowapi.ProwJob, buildID, outDir string) (*v1.Pod, error) {
	pod, err := decorate.ProwJobToPodLocal(pj, buildID, outDir)
	if err != nil {
		return nil, err
	}

	// Prompt for emptyDir or hostPath replacements for all volume sources besides those two.
	volsToFix := nonLocalVolumes(pod.Spec.Volumes)
	if len(volsToFix) > 0 {
		prompt := `For each of the following volumes specify one of:
 - 'empty' to use an emptyDir;
 - a path on the host to use hostPath;
 - '' (nothing) to use the existing volume source and assume it is available in the cluster`
		fmt.Fprintln(os.Stderr, prompt)
		for _, vol := range volsToFix {
			fmt.Fprintf(os.Stderr, "Volume %q: ", vol.Name)

			var choice string
			fmt.Scanln(&choice)
			choice = strings.TrimSpace(choice)
			switch {
			case choice == "":
				// Leave the VolumeSource as is.
			case choice == "empty" || strings.ToLower(choice) == "emptydir":
				vol.VolumeSource = v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
			default:
				vol.VolumeSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: choice}}
			}
		}
	}

	return pod, nil
}

func nonLocalVolumes(vols []v1.Volume) []*v1.Volume {
	var res []*v1.Volume
	for i, vol := range vols {
		if vol.HostPath == nil && vol.EmptyDir == nil {
			res = append(res, &vols[i])
		}
	}
	return res
}
