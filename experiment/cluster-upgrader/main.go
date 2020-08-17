/*
Copyright 2019 The Kubernetes Authors.

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
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

type options struct {
	project string
	zone    string
	cluster string
	master  string
	pools   string
	ceiling string
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.StringVar(&o.project, "project", "", "GCP project of cluster")
	flags.StringVar(&o.zone, "zone", "", "GCP zone of cluster")
	flags.StringVar(&o.cluster, "cluster", "", "GCP cluster name to upgrade")
	flags.StringVar(&o.master, "master", "", "Force target master version instead of latest")
	flags.StringVar(&o.pools, "pools", "", "Force target node pool version instead of latest")
	flags.StringVar(&o.ceiling, "ceiling", "", "Limit to versions < this one when set, so --ceiling=1.15.0 match anything less than 1.15.0")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if o.cluster == "" {
		return errors.New("empty --cluster")
	}
	return nil
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("Invalid flags")
	}
	return o
}

func main() {
	opt := parseOptions()
	log := logrus.WithFields(logrus.Fields{
		"project": opt.project,
		"zone":    opt.zone,
		"cluster": opt.cluster,
	})

	var ceil *semver.Version
	if opt.ceiling != "" {
		var err error
		if ceil, err = parse(opt.ceiling); err != nil {
			logrus.WithError(err).Fatal("Bad --ceiling")
		}
	}
	masterGoal, poolGoal, err := versions(opt.project, opt.zone, ceil)
	if err != nil {
		log.WithError(err).Fatal("Cannot find available versions")
	}
	if opt.master != "" {
		if masterGoal, err = parse(opt.master); err != nil {
			log.WithError(err).Fatal("Bad --master")
		}
	}
	if opt.pools != "" {
		if poolGoal, err = parse(opt.pools); err != nil {
			log.WithError(err).Fatal("Bad --pool")
		}
	}

	log = log.WithFields(logrus.Fields{
		"masterGoal": masterGoal,
		"poolGoal":   poolGoal,
	})

	if err := upgradeMaster(opt.project, opt.zone, opt.cluster, *masterGoal); err != nil {
		log.WithError(err).Fatal("Could not upgrade master")
	}

	log.Info("Master at goal")

	pools, err := pools(opt.project, opt.zone, opt.cluster)
	if err != nil {
		log.WithError(err).Fatal("Failed to list node pools")
	}

	baseLog := log
	for _, pool := range pools {
		log := log.WithField("pool", pool)
		if err != nil {
			log.WithError(err).Fatal("Could not determine current pool version")
		}
		if err := upgradePool(opt.project, opt.zone, opt.cluster, pool, *poolGoal); err != nil {
			log.WithError(err).Fatal("Could not upgrade pool")
		}
		log.Info("Pool at goal")
	}

	baseLog.Info("Success")
}

// versions returns the available (master, node) versions for the zone
func versions(project, zone string, ceiling *semver.Version) (*semver.Version, *semver.Version, error) {
	out, err := output(
		"gcloud", "container", "get-server-config",
		"--project="+project, "--zone="+zone,
		"--format=value(validMasterVersions,validNodeVersions)",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("get-server-config: %w", err)
	}
	parts := strings.Split(out, "\t")
	master, err := selectVersion(ceiling, strings.Split(parts[0], ";")...)
	if err != nil {
		return nil, nil, fmt.Errorf("select master version: %w", err)
	}
	pool, err := selectVersion(ceiling, strings.Split(parts[0], ";")...)
	if err != nil {
		return nil, nil, fmt.Errorf("select pool version: %w", err)
	}
	return master, pool, nil
}

// selectVersion chooses the largest first value (less than ceiling if set)
func selectVersion(ceiling *semver.Version, values ...string) (*semver.Version, error) {
	for _, val := range values {
		ver, err := parse(val)
		if err != nil {
			return nil, fmt.Errorf("bad version %s: %w", val, err)
		}
		if ceiling != nil && ver.GTE(*ceiling) {
			continue
		}
		return ver, nil
	}
	return nil, errors.New("no matches found")
}

// upgradeMaster upgrades the master to the specified version, one minor version at a time.
func upgradeMaster(project, zone, cluster string, want semver.Version) error {
	doUpgrade := func(goal string) error {
		return run(
			"gcloud", "container", "clusters", "upgrade",
			"--project="+project, "--zone="+zone, cluster, "--master",
			"--cluster-version="+goal,
		)
	}

	getVersion := func() (*semver.Version, error) {
		return masterVersion(project, zone, cluster)
	}
	return upgrade(want, doUpgrade, getVersion)
}

// masterVersion returns the current master version.
func masterVersion(project, zone, cluster string) (*semver.Version, error) {
	out, err := output(
		"gcloud", "container", "clusters", "describe",
		"--project="+project, "--zone="+zone, cluster,
		"--format=value(currentMasterVersion)",
	)
	if err != nil {
		return nil, fmt.Errorf("clusters describe: %w", err)
	}
	return parse(out)
}

// pools returns the current set of pools in the cluster.
func pools(project, zone, cluster string) ([]string, error) {
	out, err := output(
		"gcloud", "container", "node-pools", "list",
		"--project="+project, "--zone="+zone, "--cluster="+cluster,
		"--format=value(name)",
	)
	if err != nil {
		return nil, fmt.Errorf("node-pools list: %w", err)
	}
	return strings.Split(out, "\n"), nil
}

// upgradePool upgrades the pool to the specified version, one minor version at a time.
func upgradePool(project, zone, cluster, pool string, want semver.Version) error {
	doUpgrade := func(goal string) error {
		return run(
			"gcloud", "container", "clusters", "upgrade",
			"--project="+project, "--zone="+zone, cluster, "--node-pool="+pool,
			"--cluster-version="+goal,
		)
	}

	getVersion := func() (*semver.Version, error) {
		return poolVersion(project, zone, cluster, pool)
	}

	return upgrade(want, doUpgrade, getVersion)
}

// poolVersion returns the current version of the pool.
func poolVersion(project, zone, cluster, pool string) (*semver.Version, error) {
	out, err := output(
		"gcloud", "container", "node-pools", "describe",
		"--project="+project, "--zone="+zone, "--cluster="+cluster, pool,
		"--format=value(version)",
	)
	if err != nil {
		return nil, fmt.Errorf("node-pools describe: %w", err)
	}
	return parse(out)
}

type upgrader func(string) error
type versioner func() (*semver.Version, error)

func upgrade(want semver.Version, doUpgrade upgrader, getVersion versioner) error {
	for {
		have, err := getVersion()
		if err != nil {
			return fmt.Errorf("get version: %w", err)
		}
		if have.Equals(want) {
			return nil
		}
		if have.Major != want.Major {
			return fmt.Errorf("cannot change major version %d to %d", have.Major, want.Major)
		}
		var goal string
		switch {
		case have.Minor == want.Minor:
			goal = want.String()
		case have.Minor > want.Minor:
			goal = fmt.Sprintf("%d.%d", have.Major, have.Minor-1)
		default:
			goal = fmt.Sprintf("%d.%d", have.Major, have.Minor+1)
		}
		if err := doUpgrade(goal); err != nil {
			return fmt.Errorf("upgrade to %s: %w", goal, err)
		}
	}
}

// output returns the output and prints Stderr to screen.
func output(command string, args ...string) (string, error) {
	logrus.WithFields(logrus.Fields{
		"command": command,
		"args":    args,
	}).Debug("Grabbing output")
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	buf, err := cmd.Output()
	return string(buf), err
}

// run the command, printing stdout, stderr to screen.
func run(command string, args ...string) error {
	logrus.WithFields(logrus.Fields{
		"command": command,
		"args":    args,
	}).Info("Running command")
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// parse converts the string into a semver struct
func parse(out string) (*semver.Version, error) {
	ver, err := semver.Parse(strings.TrimSpace(out))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &ver, nil
}
