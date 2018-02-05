package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/golang/glog"
)

type JSONJob struct {
	Scenario string   `json:"scenario"`
	Args     []string `json:"args"`
}

type JobInfo struct {
	Name    string
	Inputs  []string
	Outputs []string
}

type NodeInfo struct {
	Key   string
	Label string
	Shape string
}

func (n *NodeInfo) ToDot() string {
	return fmt.Sprintf("%q [label=%q shape=%q];", n.Key, n.Label, n.Shape)
}

type Graph struct {
	Nodes map[string]*NodeInfo
}

func (g *Graph) GetNode(key string, label string, shape string) *NodeInfo {
	i := g.Nodes[key]
	if i == nil {
		i = &NodeInfo{
			Key:   key,
			Label: label,
			Shape: shape,
		}
		g.Nodes[key] = i
	}
	return i
}

func main() {
	flag.Set("alsologtostderr", "1")
	flag.Parse()

	err := run()
	if err != nil {
		glog.Fatalf("unexpected error: %v", err)
	}
}

func run() error {
	p := "jobs/config.json"
	config, err := ioutil.ReadFile(p)
	if err != nil {
		return fmt.Errorf("error reading %q: %v", p, err)
	}

	jobs := make(map[string]JSONJob)
	if err := json.Unmarshal(config, &jobs); err != nil {
		return fmt.Errorf("error parsing %q: %v", p, err)
	}

	jobInfos := make(map[string]*JobInfo)
	for k, j := range jobs {
		ji := &JobInfo{
			Name: k,
		}

		keys := make(map[string]string)

		for _, arg := range j.Args {
			arg = strings.TrimSpace(arg)
			if strings.HasPrefix(arg, "--env-file=") {
				envFile := strings.TrimPrefix(arg, "--env-file=")
				s, err := ioutil.ReadFile(envFile)
				if err != nil {
					return fmt.Errorf("error reading %q: %v", envFile, err)
				}

				for _, line := range strings.Split(string(s), "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.HasPrefix(line, "#") {
						continue
					}

					tokens := strings.SplitN(line, "=", 2)
					if len(tokens) != 2 {
						return fmt.Errorf("cannot parse line %q", line)
					}
					keys[tokens[0]] = tokens[1]
				}
			}

			if strings.HasPrefix(arg, "--env=") {
				line := strings.TrimPrefix(arg, "--env=")
				tokens := strings.SplitN(line, "=", 2)
				if len(tokens) != 2 {
					return fmt.Errorf("cannot parse line %q", line)
				}
				keys[tokens[0]] = tokens[1]
			}

			if strings.HasPrefix(arg, "--kops-version=") {
				kopsVersion := strings.TrimPrefix(arg, "--kops-version=")
				keys["--kops-version"] = kopsVersion
			}

			// "ci-kubernetes-e2e-kops-aws-updown": {
			// 	"args": [
			// 	  "--cluster=e2e-kops-aws-updown.test-cncf-aws.k8s.io",
			// 	  "--deployment=kops",
			// 	  "--env-file=jobs/platform/kops_aws.env",
			// 	  "--extract=ci/latest",
			// 	  "--ginkgo-parallel",
			// 	  "--kops-publish=gs://kops-ci/bin/latest-ci-updown-green.txt",
			// 	  "--kops-version=https://storage.googleapis.com/kops-ci/bin/latest-ci.txt",
			// 	  "--provider=aws",
			// 	  "--test_args=--ginkgo.flakeAttempts=2 --ginkgo.focus=\\[k8s.io\\]\\sNetworking.*\\[Conformance\\]",
			// 	  "--timeout=30m"
			// 	],

			if strings.HasPrefix(arg, "--kops-publish=") {
				kopsVersion := strings.TrimPrefix(arg, "--kops-publish=")
				keys["--kops-publish"] = kopsVersion
			}
		}

		for k, v := range keys {

			switch k {
			case "KOPS_PUBLISH_GREEN_PATH":
				ji.Outputs = append(ji.Outputs, normalizePath(v))

			case "KOPS_LATEST":
				ji.Inputs = append(ji.Inputs, normalizePath(v))

			case "KOPS_DEPLOY_LATEST_URL":
				ji.Inputs = append(ji.Inputs, normalizePath(v))

			case "--kops-version":
				ji.Inputs = append(ji.Inputs, normalizePath(v))

			case "--kops-publish":
				ji.Outputs = append(ji.Outputs, normalizePath(v))
			}
		}

		if k == "ci-kops-build" {
			// TODO: Add to config.json somehow?
			ji.Outputs = append(ji.Outputs, "gs://kops-ci/bin/latest-ci.txt")
		}

		jobInfos[k] = ji
	}

	g := &Graph{
		Nodes: make(map[string]*NodeInfo),
	}

	for _, info := range jobInfos {
		if len(info.Inputs) == 0 && len(info.Outputs) == 0 {
			// Avoid crowding the graph out with nodes where we have no info
			continue
		}

		g.GetNode(info.Name, "job "+info.Name, "circle")
		for _, s := range info.Inputs {
			g.GetNode(s, "version "+s, "box")
		}
		for _, s := range info.Outputs {
			g.GetNode(s, "version "+s, "box")
		}
	}

	fmt.Printf("digraph {\n")
	for _, info := range g.Nodes {
		fmt.Printf("%s\n", info.ToDot())
	}

	for _, info := range jobInfos {
		for _, s := range info.Inputs {
			fmt.Printf("  %q -> %q;\n", s, info.Name)
		}
		for _, s := range info.Outputs {
			fmt.Printf("  %q -> %q;\n", info.Name, s)
		}
	}

	fmt.Printf("}\n")

	return nil
}

func normalizePath(v string) string {
	if !strings.Contains(v, "://") {
		v = "gs://kops-ci/bin/" + v
	}
	if strings.HasPrefix(v, "https://storage.googleapis.com/") {
		v = "gs://" + strings.TrimPrefix(v, "https://storage.googleapis.com/")
	}
	return v
}

// func (j *JobInfo) ParseEnvFile(p string) error {
// 	s, err := ioutil.ReadFile(p)
// 	if err != nil {
// 		return fmt.Errorf("error reading %q: %v", p, err)
// 	}

// 	for _, line := range strings.Split(string(s), "\n") {
// 		line = strings.TrimSpace(line)
// 		if line == "" {
// 			continue
// 		}
// 		if strings.HasPrefix(line, "#") {
// 			continue
// 		}

// 		err := j.ParseEnvLine(line)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func (j *JobInfo) ParseEnvLine(line string) error {
// 	tokens := strings.SplitN(line, "=", 2)
// 	if len(tokens) != 2 {
// 		return fmt.Errorf("cannot parse line %q", line)
// 	}

// 	err := j.ParseEnv(tokens[0], tokens[1])
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// func (j *JobInfo) ParseEnv(k, v string) error {
// 	if k == "KOPS_PUBLISH_GREEN_PATH" {
// 		j.Outputs = append(j.Outputs, v)
// 	}

// 	if k == "KOPS_LATEST" {
// 		j.Inputs = append(j.Inputs, v)
// 	}

// 	if k == "KOPS_DEPLOY_LATEST_URL" {
// 		j.Inputs = append(j.Inputs, v)
// 	}

// 	return nil
// }
