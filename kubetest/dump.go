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
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// logDumper gets all the nodes from a kubernetes cluster and dumps a well-known set of logs
type logDumper struct {
	sshClientFactory sshClientFactory

	artifactsDir string

	services []string
	files    []string
}

// newLogDumper is the constructor for a logDumper
func newLogDumper(sshClientFactory sshClientFactory, artifactsDir string) (*logDumper, error) {
	d := &logDumper{
		sshClientFactory: sshClientFactory,
		artifactsDir:     artifactsDir,
	}

	d.services = []string{
		"node-problem-detector",
		"kubelet",
		"docker",
		"kops-configuration",
		"protokube",
	}
	d.files = []string{
		"kube-apiserver",
		"kube-scheduler",
		"rescheduler",
		"kube-controller-manager",
		"etcd",
		"etcd-events",
		"glbc",
		"cluster-autoscaler",
		"kube-addon-manager",
		"fluentd",
		"kube-proxy",
		"node-problem-detector",
		"cloud-init-output",
		"startupscript",
		"kern",
		"docker",
	}

	return d, nil
}

// DumpAllNodes connects to every node and dumps the logs
func (d *logDumper) DumpAllNodes(ctx context.Context) error {
	nodes, err := kubectlGetNodes()
	if err != nil {
		return err
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]

		host := ""
		for _, address := range node.Status.Addresses {
			if address.Type == "ExternalIP" {
				host = address.Address
				break
			}
		}

		if host == "" {
			log.Printf("could not find address for %v", node.Metadata.Name)
			continue
		}

		log.Printf("Dumping node %s", node.Metadata.Name)

		n, err := d.connectToNode(ctx, node.Metadata.Name, host)
		if err != nil {
			log.Printf("could not connect to %s (%s): %v", node.Metadata.Name, host, err)
			continue
		}

		errors := n.dump(ctx)
		for _, e := range errors {
			log.Printf("error dumping %s: %v", node.Metadata.Name, e)
		}

		if err := n.Close(); err != nil {
			log.Printf("error closing connection to %s: %v", node.Metadata.Name, err)
		}
	}

	return nil
}

// sshClient is an interface abstracting *ssh.Client, which allows us to test it
type sshClient interface {
	io.Closer

	// ExecPiped runs the command, piping stdout & stderr
	ExecPiped(ctx context.Context, command string, stdout io.Writer, stderr io.Writer) error
}

// sshClientFactory is an interface abstracting to a node over SSH
type sshClientFactory interface {
	Dial(ctx context.Context, host string) (sshClient, error)
}

// logDumperNode holds state for a particular node we are dumping
type logDumperNode struct {
	client sshClient
	dumper *logDumper

	dir string
}

// connectToNode makes an SSH connection to the node and returns a logDumperNode
func (d *logDumper) connectToNode(ctx context.Context, nodeName string, host string) (*logDumperNode, error) {
	client, err := d.sshClientFactory.Dial(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("unable to SSH to %q: %v", host, err)
	}
	return &logDumperNode{
		client: client,
		dir:    filepath.Join(d.artifactsDir, nodeName),
		dumper: d,
	}, nil
}

// logDumperNode cleans up any state in the logDumperNode
func (n *logDumperNode) Close() error {
	return n.client.Close()
}

// dump captures the well-known set of logs
func (n *logDumperNode) dump(ctx context.Context) []error {
	var errors []error

	// Capture kernel log
	if err := n.shellToFile(ctx, "sudo journalctl --output=short-precise -k", filepath.Join(n.dir, "kern.log")); err != nil {
		errors = append(errors, err)
	}

	// Capture logs from any systemd services in our list, that are registered
	services, err := n.listSystemdUnits(ctx)
	if err != nil {
		errors = append(errors, fmt.Errorf("error listing systemd services: %v", err))
	}
	for _, s := range n.dumper.services {
		name := s + ".service"
		for _, service := range services {
			if service == name {
				if err := n.shellToFile(ctx, "sudo journalctl --output=cat -u "+name, filepath.Join(n.dir, s+".log")); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}

	// Capture any file logs where the files exist
	fileList, err := n.findFiles(ctx, "/var/log")
	if err != nil {
		errors = append(errors, fmt.Errorf("error reading /var/log: %v", err))
	}
	for _, name := range n.dumper.files {
		prefix := "/var/log/" + name + ".log"
		for _, f := range fileList {
			if !strings.HasPrefix(f, prefix) {
				continue
			}
			if err := n.shellToFile(ctx, "sudo cat "+f, filepath.Join(n.dir, filepath.Base(f))); err != nil {
				errors = append(errors, err)
			}
		}
	}

	return errors
}

// findFiles lists files under the specified directory (recursively)
func (n *logDumperNode) findFiles(ctx context.Context, dir string) ([]string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := n.client.ExecPiped(ctx, "sudo find "+dir+" -print0", &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("error listing %q: %v", dir, err)
	}

	paths := []string{}
	for _, b := range bytes.Split(stdout.Bytes(), []byte{0}) {
		if len(b) == 0 {
			// Likely the last value
			continue
		}
		paths = append(paths, string(b))
	}
	return paths, nil
}

// listSystemdUnits returns the list of systemd units on the node
func (n *logDumperNode) listSystemdUnits(ctx context.Context) ([]string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := n.client.ExecPiped(ctx, "sudo systemctl list-units -t service --no-pager --no-legend --all", &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("error listing systemd units: %v", err)
	}

	var services []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		tokens := strings.Fields(line)
		if len(tokens) == 0 || tokens[0] == "" {
			continue
		}
		services = append(services, tokens[0])
	}
	return services, nil
}

// shellToFile executes a command and copies the output to a file
func (n *logDumperNode) shellToFile(ctx context.Context, command string, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		log.Printf("unable to mkdir on %q: %v", filepath.Dir(destPath), err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("error creating file %q: %v", destPath, err)
	}
	defer f.Close()

	if err := n.client.ExecPiped(ctx, command, f, f); err != nil {
		return fmt.Errorf("error executing command %q: %v", command, err)
	}

	return nil
}

// sshClientImplementation is the default implementation of sshClient, binding to a *ssh.Client
type sshClientImplementation struct {
	client *ssh.Client
}

var _ sshClient = &sshClientImplementation{}

// ExecPiped implements sshClientImplementation::ExecPiped
func (s *sshClientImplementation) ExecPiped(ctx context.Context, cmd string, stdout io.Writer, stderr io.Writer) error {
	finished := make(chan error)
	go func() {
		session, err := s.client.NewSession()
		if err != nil {
			finished <- fmt.Errorf("error creating ssh session: %v", err)
			return
		}
		defer session.Close()

		if verbose {
			log.Printf("Running SSH command: %v", cmd)
		}

		session.Stdout = stdout
		session.Stderr = stderr

		finished <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		log.Print("closing SSH tcp connection due to context completion")

		// terminate the TCP connection to force a disconnect - we assume everyone is using the same context.
		// We could make this better by sending a signal on the session, waiting and then closing the session,
		// and only if we still haven't succeeded then closing the TCP connection.  This is sufficient for our
		// current usage though - and hopefully that logic will be implemented in the SSH package itself.
		s.Close()

		<-finished // Wait for cancellation
		return ctx.Err()

	case err := <-finished:
		return err
	}
}

// Close implements sshClientImplementation::Close
func (s *sshClientImplementation) Close() error {
	return s.client.Close()
}

// sshClientFactoryImplementation is the default implementation of sshClientFactory
type sshClientFactoryImplementation struct {
	sshConfig *ssh.ClientConfig
}

var _ sshClientFactory = &sshClientFactoryImplementation{}

// Dial implements sshClientFactory::Dial
func (f *sshClientFactoryImplementation) Dial(ctx context.Context, host string) (sshClient, error) {
	addr := host + ":22"
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// We have a TCP connection; we will force-close it to support context cancellation

	var client *ssh.Client
	finished := make(chan error)
	go func() {
		c, chans, reqs, err := ssh.NewClientConn(conn, addr, f.sshConfig)
		if err == nil {
			client = ssh.NewClient(c, chans, reqs)
		}
		finished <- err
	}()

	select {
	case <-ctx.Done():
		log.Print("cancelling SSH tcp connection due to context completion")
		conn.Close() // Close the TCP connection to force cancellation
		<-finished   // Wait for cancellation
		return nil, ctx.Err()
	case err := <-finished:
		if err != nil {
			return nil, err
		}
		return &sshClientImplementation{
			client: client,
		}, nil
	}
}
