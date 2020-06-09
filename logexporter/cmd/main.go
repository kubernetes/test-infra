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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog"
)

// Initialize the log exporter's configuration related flags.
var (
	cloudProvider        = pflag.String("cloud-provider", "", "Cloud provider for this node (gce/gke/aws/kubemark/..)")
	dumpSystemdJournal   = pflag.Bool("dump-systemd-journal", false, "Whether to dump the full systemd journal")
	enableHollowNodeLogs = pflag.Bool("enable-hollow-node-logs", false, "Enable uploading hollow node logs too. Relevant only for kubemark nodes")
	extraLogFiles        = pflag.StringSlice("extra-log-files", []string{}, "Extra log files to dump")
	extraSystemdServices = pflag.StringSlice("extra-systemd-services", []string{}, "Extra systemd services to dump")
	gcsPath              = pflag.String("gcs-path", "", "Path to the GCS directory under which to upload logs, for eg: gs://my-logs-bucket/logs")
	gcloudAuthFilePath   = pflag.String("gcloud-auth-file-path", "/etc/service-account/service-account.json", "Path to gcloud service account file, for authenticating gsutil to write to GCS bucket")
	journalPath          = pflag.String("journal-path", "/var/log/journal", "Path where the systemd journal dir is mounted")
	nodeName             = pflag.String("node-name", "", "Name of the node this log exporter is running on")
	sleepDuration        = pflag.Duration("sleep-duration", 60*time.Second, "Duration to sleep before exiting with success. Useful for making pods schedule with hard anti-affinity when run as a job on a k8s cluster")
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
	klog.Info("Verifying if a valid config has been provided through the flags")
	if *nodeName == "" {
		return fmt.Errorf("Flag --node-name has its value unspecified")
	}
	if *gcsPath == "" {
		return fmt.Errorf("Flag --gcs-path has its value unspecified")
	}
	if _, err := os.Stat(*gcloudAuthFilePath); err != nil {
		return fmt.Errorf("Could not find the gcloud service account file: %v", err)
	} else if err := runCommand("gcloud", "auth", "activate-service-account", "--key-file="+*gcloudAuthFilePath); err != nil {
		return fmt.Errorf("Failed to activate gcloud service account: %v", err)
	}
	return nil
}

// Create logfile for systemd service in outputDir with the given journalctl outputMode.
func createSystemdLogfile(service string, outputMode string, outputDir string) error {
	// Generate the journalctl command.
	journalCmdArgs := []string{fmt.Sprintf("--output=%v", outputMode), "-D", *journalPath}
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

// createFullSystemdLogfile creates logfile for full systemd journal in the outputDir.
func createFullSystemdLogfile(outputDir string) error {
	cmd := exec.Command("journalctl", "--output=short-precise", "-D", *journalPath)
	// Run the command and record the output to a file.
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Journalctl command failed: %v", err)
	}
	logfile := filepath.Join(outputDir, "systemd.log")
	if err := ioutil.WriteFile(logfile, output, 0444); err != nil {
		return fmt.Errorf("Writing full journalctl logs to file failed: %v", err)
	}
	return nil
}

// Create logfiles for systemd services in outputDir.
func createSystemdLogfiles(outputDir string) {
	services := append(systemdServices, nodeSystemdServices...)
	services = append(services, *extraSystemdServices...)
	for _, service := range services {
		if err := createSystemdLogfile(service, "cat", outputDir); err != nil {
			klog.Warningf("Failed to record journalctl logs: %v", err)
		}
	}
	// Service logs specific to VM setup.
	for _, service := range systemdSetupServices {
		if err := createSystemdLogfile(service, "short-precise", outputDir); err != nil {
			klog.Warningf("Failed to record journalctl logs: %v", err)
		}
	}
	if *dumpSystemdJournal {
		if err := createFullSystemdLogfile(outputDir); err != nil {
			klog.Warningf("Failed to record journalctl logs: %v", err)
		}
	}
}

// Copy logfiles specific to this node based on the cloud-provider, system services, etc
// to a temporary directory. Also create logfiles for systemd services if journalctl is present.
// We do not expect this function to see an error.
func prepareLogfiles(logDir string) {
	klog.Info("Preparing logfiles relevant to this node")
	logfiles := nodeLogs[:]
	logfiles = append(logfiles, *extraLogFiles...)

	switch *cloudProvider {
	case "gce", "gke":
		logfiles = append(logfiles, gceLogs...)
	case "aws":
		logfiles = append(logfiles, awsLogs...)
	default:
		klog.Errorf("Unknown cloud provider '%v' provided, skipping any provider specific logs", *cloudProvider)
	}

	// Grab kubemark logs too, if asked for.
	if *enableHollowNodeLogs {
		logfiles = append(logfiles, kubemarkLogs...)
	}

	// Select system/service specific logs.
	if _, err := os.Stat("/workspace/etc/systemd/journald.conf"); err == nil {
		klog.Info("Journalctl found on host. Collecting systemd logs")
		createSystemdLogfiles(logDir)
	} else {
		klog.Infof("Journalctl not found on host (%v). Collecting supervisord logs instead", err)
		logfiles = append(logfiles, kernelLog)
		logfiles = append(logfiles, initdLogs...)
		logfiles = append(logfiles, supervisordLogs...)
	}

	// Copy all the logfiles that exist, to logDir.
	for _, logfile := range logfiles {
		logfileFullPath := filepath.Join(localLogPath, logfile+".log*") // Append .log* to copy rotated logs too.
		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cp %v %v", logfileFullPath, logDir))
		if err := cmd.Run(); err != nil {
			klog.Warningf("Failed to copy any logfiles with pattern '%v': %v", logfileFullPath, err)
		}
	}
}

func uploadLogfilesToGCS(logDir string) error {
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("ls %v/*", logDir))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Could not list any logfiles: %v", err)
	}
	klog.Infof("List of logfiles available: %v", string(output))

	gcsLogPath := *gcsPath + "/" + *nodeName
	klog.Infof("Uploading logfiles to GCS at path '%v'", gcsLogPath)
	for uploadAttempt := 0; uploadAttempt < 3; uploadAttempt++ {
		// Upload the files with compression (-z) and parallelism (-m) for speeding
		// up, and set their ACL to make them publicly readable.
		if err = runCommand("gsutil", "-m", "-q", "cp", "-a", "public-read", "-c",
			"-z", "log,txt,xml", logDir+"/*", gcsLogPath); err != nil {
			klog.Errorf("Attempt %v to upload to GCS failed: %v", uploadAttempt, err)
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

func runCommand(name string, arg ...string) error {
	klog.Infof("Running: %s %s", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	var stderr, stdout bytes.Buffer
	cmd.Stderr, cmd.Stdout = &stderr, &stdout
	err := cmd.Run()
	klog.Infof("Stdout:\n%s\n", stdout.String())
	klog.Infof("Stderr:\n%s\n", stderr.String())
	return err
}

func dumpNetworkDebugInfo() {
	klog.Info("Dumping network connectivity debug info")
	resolv, err := ioutil.ReadFile("/etc/resolv.conf")
	if err != nil {
		klog.Errorf("Failed to read /etc/resolv.conf: %v", err)
	}
	klog.Infof("/etc/resolv.conf: %q", string(resolv))
	addrs, err := net.LookupHost("kubernetes.default")
	if err != nil {
		klog.Errorf("Failed to resolve kubernetes.default: %v", err)
	}
	klog.Infof("kubernetes.default resolves to: %v", addrs)
	addrs, err = net.LookupHost("google.com")
	if err != nil {
		klog.Errorf("Failed to resolve google.com: %v", err)
	}
	klog.Infof("google.com resolves to: %v", addrs)
	resp, err := http.Get("http://google.com/")
	if err != nil {
		klog.Errorf("Failed to get http://google.com/: %v", err)
	}
	defer resp.Body.Close()
	klog.Infof("GET http://google.com finished with: %v code", resp.StatusCode)
}

func main() {
	pflag.Parse()
	if err := checkConfigValidity(); err != nil {
		klog.Errorf("Bad config provided: %v", err)
		dumpNetworkDebugInfo()
		klog.Fatalf("Bad config provided: %v", err)
	}

	localTmpLogPath, err := ioutil.TempDir("/tmp", "k8s-systemd-logs")
	if err != nil {
		klog.Fatalf("Could not create temporary dir locally for copying logs: %v", err)
	}
	defer os.RemoveAll(localTmpLogPath)

	prepareLogfiles(localTmpLogPath)
	if err := uploadLogfilesToGCS(localTmpLogPath); err != nil {
		klog.Fatalf("Could not upload logs to GCS: %v", err)
	}
	klog.Info("Logs successfully uploaded")

	klog.Infof("Entering sleep for a duration of %v seconds", *sleepDuration)
	time.Sleep(*sleepDuration)
}
