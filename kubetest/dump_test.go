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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func Test_logDumperNode_findFiles(t *testing.T) {
	grid := []struct {
		dir      string
		command  string
		stdout   string
		stderr   string
		expected []string
	}{
		{
			dir:     "/var/log",
			command: "sudo find /var/log -print0",
			stdout:  "/var/log/a\x00/var/log/b\x00",
			expected: []string{
				"/var/log/a",
				"/var/log/b",
			},
		},
		{
			dir:     "/var/log",
			command: "sudo find /var/log -print0",
			stdout:  "/var/log/a\x00",
			expected: []string{
				"/var/log/a",
			},
		},
		{
			dir:      "/var/log",
			command:  "sudo find /var/log -print0",
			expected: []string{},
		},
	}

	for _, g := range grid {
		client := &mockSSHClient{}
		n := logDumperNode{
			client: client,
		}

		client.commands = append(client.commands, &mockCommand{
			command: g.command,
			stdout:  []byte(g.stdout),
			stderr:  []byte(g.stderr),
		})
		actual, err := n.findFiles(context.Background(), g.dir)
		if err != nil {
			t.Errorf("unexpected error from findFiles: %v", err)
			continue
		}

		if !reflect.DeepEqual(actual, g.expected) {
			t.Errorf("unexpected files.  actual=%v, expected=%v", actual, g.expected)
			continue
		}
	}
}

func Test_logDumperNode_listSystemdUnits(t *testing.T) {
	grid := []struct {
		command  string
		stdout   string
		stderr   string
		expected []string
	}{
		{
			command: "sudo systemctl list-units -t service --no-pager --no-legend --all",
			stdout: "accounts-daemon.service            loaded active running Accounts Service\n" +
				"acpid.service                      loaded active running ACPI event daemon\n" +
				"atd.service                        loaded active running Deferred execution scheduler\n" +
				"avahi-daemon.service               loaded active running Avahi mDNS/DNS-SD Stack\n" +
				"bluetooth.service                  loaded active running Bluetooth service\n" +
				"cameras.service                    loaded failed failed  cameras\n" +
				"colord.service                     loaded active running Manage, Install and Generate Color Profiles\n",
			expected: []string{
				"accounts-daemon.service",
				"acpid.service",
				"atd.service",
				"avahi-daemon.service",
				"bluetooth.service",
				"cameras.service",
				"colord.service",
			},
		},
	}

	for _, g := range grid {
		client := &mockSSHClient{}
		n := logDumperNode{
			client: client,
		}

		client.commands = append(client.commands, &mockCommand{
			command: g.command,
			stdout:  []byte(g.stdout),
			stderr:  []byte(g.stderr),
		})
		actual, err := n.listSystemdUnits(context.Background())
		if err != nil {
			t.Errorf("unexpected error from listSystemdUnits: %v", err)
			continue
		}

		if !reflect.DeepEqual(actual, g.expected) {
			t.Errorf("unexpected systemdUnits.  actual=%v, expected=%v", actual, g.expected)
			continue
		}
	}
}

func Test_logDumperNode_shellToFile(t *testing.T) {
	grid := []struct {
		command string
		stdout  []byte
		stderr  []byte
	}{
		{
			command: "cat something",
			stdout:  []byte("hello"),
		},
	}

	for _, g := range grid {
		client := &mockSSHClient{}
		n := logDumperNode{
			client: client,
		}

		client.commands = append(client.commands, &mockCommand{
			command: g.command,
			stdout:  []byte(g.stdout),
			stderr:  []byte(g.stderr),
		})

		tmpfile, err := ioutil.TempFile("", "")
		if err != nil {
			t.Errorf("error creating temp file: %v", err)
			continue
		}

		defer func() {
			if err := os.Remove(tmpfile.Name()); err != nil {
				t.Errorf("error removing temp file: %v", err)
			}
		}()

		err = n.shellToFile(context.Background(), "cat something", tmpfile.Name())
		if err != nil {
			t.Errorf("unexpected error from shellToFile: %v", err)
			continue
		}

		if err := tmpfile.Close(); err != nil {
			t.Errorf("unexpected error closing file: %v", err)
			continue
		}

		actual, err := ioutil.ReadFile(tmpfile.Name())
		if err != nil {
			t.Errorf("unexpected error reading file: %v", err)
			continue
		}

		if !reflect.DeepEqual(actual, g.stdout) {
			t.Errorf("unexpected systemdUnits.  actual=%q, expected=%q", string(actual), string(g.stdout))
			continue
		}
	}
}

func Test_logDumperNode_dump(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("error creating temp dir: %v", err)
		return
	}

	defer func() {
		if err := os.RemoveAll(tmpdir); err != nil {
			t.Errorf("error removing temp dir: %v", err)
		}
	}()

	host1Client := &mockSSHClient{}
	host1Client.commands = append(host1Client.commands,
		&mockCommand{
			command: "sudo journalctl --output=short-precise -k",
		},
		&mockCommand{
			command: "sudo journalctl --output=short-precise",
		},
		&mockCommand{
			command: "sudo sysctl --all",
		},
		&mockCommand{
			command: "sudo systemctl list-units -t service --no-pager --no-legend --all",
			stdout: []byte(
				"kubelet.service                      loaded active running kubelet daemon\n" +
					"atd.service                        loaded active running Deferred execution scheduler\n" +
					"avahi-daemon.service               loaded active running Avahi mDNS/DNS-SD Stack\n" +
					"bluetooth.service                  loaded active running Bluetooth service\n" +
					"cameras.service                    loaded failed failed  cameras\n" +
					"colord.service                     loaded active running Manage, Install and Generate Color Profiles\n",
			),
		},
		&mockCommand{
			command: "sudo find /var/log -print0",
			stdout: []byte(strings.Join([]string{
				"/var/log",
				"/var/log/kube-controller-manager.log",
				"/var/log/kube-controller-manager.log.1",
				"/var/log/kube-controller-manager.log.2.gz",
				"/var/log/other.log",
			}, "\x00")),
		},
		&mockCommand{
			command: "sudo journalctl --output=cat -u kubelet.service",
		},
		&mockCommand{
			command: "sudo cat /var/log/kube-controller-manager.log",
		},
		&mockCommand{
			command: "sudo cat /var/log/kube-controller-manager.log.1",
		},
		&mockCommand{
			command: "sudo cat /var/log/kube-controller-manager.log.2.gz",
		},
	)
	mockSSHClientFactory := &mockSSHClientFactory{
		clients: map[string]sshClient{
			"host1": host1Client,
		},
	}

	dumper, err := newLogDumper(mockSSHClientFactory, tmpdir)
	if err != nil {
		t.Errorf("error building logDumper: %v", err)
	}
	dumper.DumpSysctls = true

	n, err := dumper.connectToNode(context.Background(), "nodename1", "host1")
	if err != nil {
		t.Errorf("error from connectToNode: %v", err)
	}

	errors := n.dump(context.Background())
	if len(errors) != 0 {
		t.Errorf("unexpected errors from dump: %v", errors)
		return
	}

	actual := []string{}
	err = filepath.Walk(tmpdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		p := strings.TrimPrefix(strings.TrimPrefix(path, tmpdir), "/")
		if p == "" {
			return nil
		}
		if info.IsDir() {
			p += "/"
		}
		actual = append(actual, p)
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error walking output tree: %v", err)
		return
	}

	expected := []string{
		"nodename1/",
		"nodename1/kern.log",
		"nodename1/journal.log",
		"nodename1/kubelet.log",
		"nodename1/sysctl.conf",
		"nodename1/kube-controller-manager.log",
		"nodename1/kube-controller-manager.log.1",
		"nodename1/kube-controller-manager.log.2.gz",
	}

	sort.Strings(actual)
	sort.Strings(expected)

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("unexpected files found in dump: actual=%v, expected=%v", actual, expected)
		return
	}
}

// mockCommand is an expected command and canned response
type mockCommand struct {
	command string
	stdout  []byte
	stderr  []byte
	err     error
}

// mockSSHClient is a mock implementation of sshClient
type mockSSHClient struct {
	// commands holds the canned commands and responses we expect
	commands []*mockCommand
	// closed is set if the mockSSHClient is closed
	closed bool
}

var _ sshClient = &mockSSHClient{}

// mockSSHClientFactory is a mock implementation of sshClientFactory
type mockSSHClientFactory struct {
	clients map[string]sshClient
}

var _ sshClientFactory = &mockSSHClientFactory{}

func (f *mockSSHClientFactory) Dial(ctx context.Context, host string) (sshClient, error) {
	client := f.clients[host]
	if client == nil {
		return nil, fmt.Errorf("host %q not registered in mockSSHClientFactory", host)
	}
	return client, nil
}

// Close implements sshClient::Close.  It records that the client was closed; future calls will fail
func (m *mockSSHClient) Close() error {
	if m.closed {
		return fmt.Errorf("mockSSHClient::Close called on Closed mockSSHClient")
	}
	m.closed = true
	return nil
}

// ExecPiped implements sshClient::ExecPiped.  It scans the configured commands, and returns the result if one is found.
// If no command is found, it returns an error.
func (m *mockSSHClient) ExecPiped(ctx context.Context, command string, stdout io.Writer, stderr io.Writer) error {
	if m.closed {
		return fmt.Errorf("mockSSHClient::ExecPiped called on Closed mockSSHClient")
	}
	for i := range m.commands {
		c := m.commands[i]
		if c == nil {
			continue
		}
		if c.command == command {
			if _, err := stdout.Write(c.stdout); err != nil {
				return fmt.Errorf("error writing to stdout: %v", err)
			}
			if _, err := stderr.Write(c.stderr); err != nil {
				return fmt.Errorf("error writing to stderr: %v", err)
			}
			m.commands[i] = nil
			return c.err
		}
	}

	return fmt.Errorf("unexpected command: %s", command)
}

func TestFindInstancesNotDumped(t *testing.T) {
	n1 := &node{
		Status: nodeStatus{
			Addresses: []nodeAddress{{Address: "10.0.0.1"}},
		},
	}

	n2 := &node{
		Status: nodeStatus{
			Addresses: []nodeAddress{{Address: "10.0.0.2"}},
		},
	}
	n3 := &node{
		Status: nodeStatus{
			Addresses: []nodeAddress{
				{Address: "10.0.0.3"},
				{Address: "10.0.3.3"},
			},
		},
	}

	grid := []struct {
		ips      []string
		dumped   []*node
		expected []string
	}{
		{
			ips:      nil,
			dumped:   nil,
			expected: nil,
		},
		{
			ips:      []string{"10.0.0.1"},
			dumped:   nil,
			expected: []string{"10.0.0.1"},
		},
		{
			ips:      []string{"10.0.0.1"},
			dumped:   []*node{n1},
			expected: nil,
		},
		{
			ips:      []string{"10.0.0.1", "10.0.0.2"},
			dumped:   []*node{n1},
			expected: []string{"10.0.0.2"},
		},
		{
			ips:      []string{"10.0.0.1", "10.0.0.2", "10.0.3.3"},
			dumped:   []*node{n1, n2, n3},
			expected: nil,
		},
		{
			ips:      []string{"10.0.0.1", "10.0.0.2", "10.0.3.3"},
			dumped:   []*node{n1, n2},
			expected: []string{"10.0.3.3"},
		},
	}

	for _, g := range grid {
		actual := findInstancesNotDumped(g.ips, g.dumped)

		if !reflect.DeepEqual(actual, g.expected) {
			t.Errorf("unexpected result from findInstancesNotDumped.  actual=%v, expected=%v", actual, g.expected)
		}
	}
}
