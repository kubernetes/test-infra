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

// TODO(shyamjvs): Make this exporter work for master too, currently facing
// gcloud auth error when run from within a pod on the master.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/golang/glog"
)

// Initialize the log exporter's configuration related flags.
var (
	nodeName             = flag.String("node-name", "", "Name of the node this log exporter is running on")
	gcsPath              = flag.String("gcs-path", "", "Path to the GCS directory under which to upload logs, for eg: gs://my-logs-bucket/logs")
	cloudProvider        = flag.String("cloud-provider", "", "Cloud provider for this node (gce/gke/aws/kubemark/..)")
	gcloudAuthFilePath   = flag.String("gcloud-auth-file-path", "/etc/service-account/service-account.json", "Path to gcloud service account file, for authenticating gsutil to write to GCS bucket")
	enableHollowNodeLogs = flag.Bool("enable-hollow-node-logs", false, "Enable uploading hollow node logs too. Relevant only for kubemark nodes")
	sleepDuration        = flag.Duration("sleep-duration", 60*time.Second, "Duration to sleep before exiting with success. Useful for making pods schedule with hard anti-affinity when run as a job on a k8s cluster")
)

var (
	localLogPath = "/var/log"

	// Node-type specific logfiles.
	// Currently we only handle nodes, and neglect master.
	nodeLogs = []string{"kube-proxy", "node-problem-detector", "fluentd"}

	// Cloud provider specific logfiles.
	awsLogs      = []string{"cloud-init-output"}
	gceLogs      = []string{"startupscript"}
	kubemarkLogs = []string{"*-hollow-node-*"}

	// System services/kernel related logfiles.
	kernelLog            = "kern"
	initdLogs            = []string{"docker"}
	supervisordLogs      = []string{"kubelet", "supervisor/supervisord", "supervisor/kubelet-stdout", "supervisor/kubelet-stderr", "supervisor/docker-stdout", "supervisor/docker-stderr"}
	systemdServices      = []string{"kern", "kubelet", "docker"}
	systemdSetupServices = []string{"kube-node-installation", "kube-node-configuration"}
	nodeSystemdServices  = []string{"node-problem-detector"}
)

// Check if the config provided through the flags take valid values.
func checkConfigValidity() error {
	glog.Info("Verifying if a valid config has been provided through the flags")
	if *nodeName == "" {
		return fmt.Errorf("Flag --node-name has its value unspecified")
	}
	if *gcsPath == "" {
		return fmt.Errorf("Flag --gcs-path has its value unspecified")
	}
	if _, err := os.Stat(*gcloudAuthFilePath); err != nil {
		return fmt.Errorf("Could not find the gcloud service account file: %v", err)
	} else {
		cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file="+*gcloudAuthFilePath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Failed to activate gcloud service account: %v", err)
		}
	}
	return nil
}

// Create logfile for systemd service in outputDir with the given journalctl outputMode.
func createSystemdLogfile(service string, outputMode string, outputDir string) error {
	// Generate the journalctl command.
	journalCmdArgs := []string{fmt.Sprintf("--output=%v", outputMode), "-D", "/var/log/journal"}
	if service == "kern" {
		journalCmdArgs = append(journalCmdArgs, "-k")
	} else {
		journalCmdArgs = append(journalCmdArgs, "-u", fmt.Sprintf("%v.service", service))
	}
	cmd := exec.Command("journalctl", journalCmdArgs...)

	// Run the command and record the output to a file.
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Journalctl command for '%v' service failed: %v", service, err)
	}
	logfile := filepath.Join(outputDir, service+".log")
	if err := ioutil.WriteFile(logfile, output, 0444); err != nil {
		return fmt.Errorf("Writing to file of journalctl logs for '%v' service failed: %v", service, err)
	}
	return nil
}

// Create logfiles for systemd services in outputDir.
func createSystemdLogfiles(outputDir string) {
	services := append(systemdServices, nodeSystemdServices...)
	for _, service := range services {
		if err := createSystemdLogfile(service, "cat", outputDir); err != nil {
			glog.Warningf("Failed to record journalctl logs: %v", err)
		}
	}
	// Service logs specific to VM setup.
	for _, service := range systemdSetupServices {
		if err := createSystemdLogfile(service, "short-precise", outputDir); err != nil {
			glog.Warningf("Failed to record journalctl logs: %v", err)
		}
	}
}

// Copy logfiles specific to this node based on the cloud-provider, system services, etc
// to a temporary directory. Also create logfiles for systemd services if journalctl is present.
// We do not expect this function to see an error.
func prepareLogfiles(logDir string) {
	glog.Info("Preparing logfiles relevant to this node")
	logfiles := nodeLogs[:]

	switch *cloudProvider {
	case "gce", "gke":
		logfiles = append(logfiles, gceLogs...)
	case "aws":
		logfiles = append(logfiles, awsLogs...)
	default:
		glog.Errorf("Unknown cloud provider '%v' provided, skipping any provider specific logs", *cloudProvider)
	}

	// Grab kubemark logs too, if asked for.
	if *enableHollowNodeLogs {
		logfiles = append(logfiles, kubemarkLogs...)
	}

	// Select system/service specific logs.
	if _, err := os.Stat("/workspace/etc/systemd/journald.conf"); err == nil {
		glog.Info("Journalctl found on host. Collecting systemd logs")
		createSystemdLogfiles(logDir)
	} else {
		glog.Infof("Journalctl not found on host (%v). Collecting supervisord logs instead", err)
		logfiles = append(logfiles, kernelLog)
		logfiles = append(logfiles, initdLogs...)
		logfiles = append(logfiles, supervisordLogs...)
	}

	// Copy all the logfiles that exist, to logDir.
	for _, logfile := range logfiles {
		logfileFullPath := filepath.Join(localLogPath, logfile+".log*") // Append .log* to copy rotated logs too.
		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cp %v %v", logfileFullPath, logDir))
		if err := cmd.Run(); err != nil {
			glog.Warningf("Failed to copy any logfiles with pattern '%v': %v", logfileFullPath, err)
		}
	}
}

func uploadLogfilesToGCS(logDir string) error {
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("ls %v/*", logDir))
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("Could not list any logfiles: %v", err)
	} else {
		glog.Infof("List of logfiles available: %v", string(output))
	}

	gcsLogPath := *gcsPath + "/" + *nodeName
	glog.Infof("Uploading logfiles to GCS at path '%v'", gcsLogPath)
	var err error
	for uploadAttempt := 0; uploadAttempt < 3; uploadAttempt++ {
		// Upload the files with compression (-z) and parallelism (-m) for speeding
		// up, and set their ACL to make them publicly readable.
		cmd := exec.Command("gsutil", "-m", "-q", "cp", "-a", "public-read", "-c",
			"-z", "log,txt,xml", logDir+"/*", gcsLogPath)
		if err = cmd.Run(); err != nil {
			glog.Errorf("Attempt %v to upload to GCS failed: %v", uploadAttempt, err)
			continue
		}
		return writeSuccessMarkerFile()
	}
	return fmt.Errorf("Multiple attempts of gsutil failed, the final one due to: %v", err)
}

// Write a marker file to GCS named after this node to indicate logexporter's success.
// The directory to which we write this file can then be used as a registry to quickly
// fetch the list of nodes on which logexporter succeeded.
func writeSuccessMarkerFile() error {
	markerFilePath := *gcsPath + "/logexported-nodes-registry/" + *nodeName + ".txt"
	cmd := exec.Command("gsutil", "-q", "cp", "-a", "public-read", "-", markerFilePath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("Failed to get stdin pipe to write marker file: %v", err)
	}
	io.WriteString(stdin, "")
	stdin.Close()
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("Failed to write marker file to GCS: %v", err)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := checkConfigValidity(); err != nil {
		glog.Fatalf("Bad config provided: %v", err)
	}

	localTmpLogPath, err := ioutil.TempDir("/tmp", "k8s-systemd-logs")
	if err != nil {
		glog.Fatalf("Could not create temporary dir locally for copying logs: %v", err)
	}
	defer os.RemoveAll(localTmpLogPath)

	prepareLogfiles(localTmpLogPath)
	if err := uploadLogfilesToGCS(localTmpLogPath); err != nil {
		glog.Fatalf("Could not upload logs to GCS: %v", err)
	}
	glog.Info("Logs successfully uploaded")

	glog.Infof("Entering sleep for a duration of %v seconds", *sleepDuration)
	time.Sleep(*sleepDuration)
}
