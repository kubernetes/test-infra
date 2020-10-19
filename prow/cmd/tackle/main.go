/*
Copyright 2018 The Kubernetes Authors.

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
	context2 "context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // for gcp auth provider
)

// printArray prints items in collection (up to a non-zero limit) and return a bool indicating if results were truncated.
func printArray(collection []string, limit int) bool {
	for idx, item := range collection {
		if limit > 0 && idx == limit {
			break
		}
		fmt.Println("  *", item)
	}

	if limit > 0 && len(collection) > limit {
		return true
	}

	return false
}

// validateNotEmpty handles validation that a collection is non-empty.
func validateNotEmpty(collection []string) bool {
	return len(collection) > 0
}

// validateContainment handles validation for containment of target in collection.
func validateContainment(collection []string, target string) bool {
	for _, val := range collection {
		if val == target {
			return true
		}
	}

	return false
}

// prompt prompts user with a message (and optional default value); return the selection as string.
func prompt(promptMsg string, defaultVal string) string {
	var choice string

	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", promptMsg, defaultVal)
	} else {
		fmt.Printf("%s: ", promptMsg)
	}

	fmt.Scanln(&choice)

	// If no `choice` and a `default`, then use `default`
	if choice == "" {
		return defaultVal
	}

	return choice
}

// ensure will ensure binary is on path or return an error with install message.
func ensure(binary, install string) error {
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s: %s", binary, install)
	}
	return nil
}

// ensureKubectl ensures kubectl is on path or prints a note of how to install.
func ensureKubectl() error {
	return ensure("kubectl", "gcloud components install kubectl")
}

// ensureGcloud ensures gcloud on path or prints a note of how to install.
func ensureGcloud() error {
	return ensure("gcloud", "https://cloud.google.com/sdk/gcloud")
}

// output returns the trimmed output of running args, or an err on non-zero exit.
func output(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

// currentAccount returns the configured account for gcloud
func currentAccount() (string, error) {
	return output("gcloud", "config", "get-value", "core/account")
}

// currentProject returns the configured project for gcloud
func currentProject() (string, error) {
	return output("gcloud", "config", "get-value", "core/project")
}

// currentZone returns the configured zone for gcloud
func currentZone() (string, error) {
	return output("gcloud", "config", "get-value", "compute/zone")
}

// project holds info about a project
type project struct {
	name string
	id   string
}

// projects returns the list of accessible gcp projects
func projects(max int) ([]string, error) {
	out, err := output("gcloud", "projects", "list", fmt.Sprintf("--limit=%d", max), "--format=value(project_id)")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// zones returns the list of accessible gcp zones
func zones() ([]string, error) {
	out, err := output("gcloud", "compute", "zones", "list", "--format=value(name)")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// selectProject returns the user-selected project, defaulting to the current gcloud one.
func selectProject(choice string) (string, error) {
	fmt.Print("Getting active GCP account...")
	who, err := currentAccount()
	if err != nil {
		logrus.Warn("Run gcloud auth login to initialize gcloud")
		return "", err
	}
	fmt.Println(who)

	var projs []string

	if choice == "" {
		fmt.Printf("Projects available to %s:", who)
		fmt.Println()
		const max = 20
		projs, err = projects(max)
		for _, proj := range projs {
			fmt.Println("  *", proj)
		}
		if err != nil {
			return "", fmt.Errorf("list projects: %v", err)
		}
		if len(projs) == 0 {
			fmt.Println("Create a project at https://console.cloud.google.com/")
			return "", errors.New("no projects")
		}
		if len(projs) == max {
			fmt.Println("  ... Wow, that is a lot of projects!")
			fmt.Println("Type the name of any project, including ones not in this truncated list")
		}

		def, err := currentProject()
		if err != nil {
			return "", fmt.Errorf("get current project: %v", err)
		}

		choice = prompt("Select project", def)

		// use default project
		if choice == "" {
			return def, nil
		}
	}

	// is this a project from the list?
	for _, p := range projs {
		if p == choice {
			return choice, nil
		}
	}

	fmt.Printf("Ensuring %s has access to %s...", who, choice)
	fmt.Println()

	// no, make sure user has access to it
	if err = exec.Command("gcloud", "projects", "describe", choice).Run(); err != nil {
		return "", fmt.Errorf("%s cannot describe project: %v", who, err)
	}

	return choice, nil
}

// selectZone returns the user-selected zone, defaulting to the current gcloud one.
func selectZone() (string, error) {
	const MaxZones = 20

	def, err := currentZone()
	if err != nil {
		return "", fmt.Errorf("get current zone: %v", err)
	}

	fmt.Printf("Available zones:\n")

	zoneList, err := zones()
	if err != nil {
		return "", fmt.Errorf("list zones: %v", err)
	}

	isNonEmpty := validateNotEmpty(zoneList)
	if !isNonEmpty {
		return "", errors.New("no available zones")
	}

	if def == "" {
		// Arbitrarily select the first zone as the `default` if unset
		def = zoneList[0]
	}

	isTruncated := printArray(zoneList, MaxZones)
	if isTruncated {
		fmt.Println("  ...")
		fmt.Println("Type the name of any zone, including ones not in this truncated list")
	}

	choice := prompt("Select zone", def)

	isContained := validateContainment(zoneList, choice)
	if !isContained {
		return "", fmt.Errorf("invalid zone selection: %v", choice)
	}

	return choice, nil
}

// cluster holds info about a GKE cluster
type cluster struct {
	name    string
	zone    string
	project string
}

func (c cluster) context() string {
	return fmt.Sprintf("gke_%s_%s_%s", c.project, c.zone, c.name)
}

// currentClusters returns a {name: cluster} map.
func currentClusters(proj string) (map[string]cluster, error) {
	clusters, err := output("gcloud", "container", "clusters", "list", "--project="+proj, "--format=value(name,zone)")
	if err != nil {
		return nil, fmt.Errorf("list clusters: %v", err)
	}
	options := map[string]cluster{}
	for _, line := range strings.Split(clusters, "\n") {
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad line: %q", line)
		}
		c := cluster{name: parts[0], zone: parts[1], project: proj}
		options[c.name] = c
	}
	return options, nil
}

// createCluster causes gcloud to create a cluster in project, returning the context name
func createCluster(proj, choice string) (*cluster, error) {
	const def = "prow"
	if choice == "" {
		choice = prompt("Cluster name", def)
	}

	zone, err := selectZone()
	if err != nil {
		return nil, fmt.Errorf("select current zone for cluster: %v", err)
	}

	cmd := exec.Command("gcloud", "container", "clusters", "create", choice, "--zone="+zone, "--enable-legacy-authorization", "--issue-client-certificate")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("create cluster: %v", err)
	}

	out, err := output("gcloud", "container", "clusters", "describe", choice, "--zone="+zone, "--format=value(name,zone)")
	if err != nil {
		return nil, fmt.Errorf("describe cluster: %v", err)
	}
	parts := strings.Split(out, "\t")
	if len(parts) != 2 {
		return nil, fmt.Errorf("bad describe cluster output: %s", out)
	}

	return &cluster{name: parts[0], zone: parts[1], project: proj}, nil
}

// createContext has the user create a context.
func createContext(co contextOptions) (string, error) {
	proj, err := selectProject(co.project)
	if err != nil {
		logrus.Info("Run gcloud auth login to initialize gcloud")
		return "", fmt.Errorf("get current project: %v", err)
	}

	fmt.Printf("Existing GKE clusters in %s:", proj)
	fmt.Println()
	clusters, err := currentClusters(proj)
	if err != nil {
		return "", fmt.Errorf("list %s clusters: %v", proj, err)
	}
	for name := range clusters {
		fmt.Println("  *", name)
	}
	if len(clusters) == 0 {
		fmt.Println("  No clusters")
	}
	var choice string
	create := co.create
	reuse := co.reuse
	switch {
	case create != "" && reuse != "":
		return "", errors.New("Cannot use both --create and --reuse")
	case create != "":
		fmt.Println("Creating new " + create + " cluster...")
		choice = "new"
	case reuse != "":
		fmt.Println("Reusing existing " + reuse + " cluster...")
		choice = reuse
	default:
		choice = prompt("Get credentials for existing cluster or", "create new")
	}

	if choice == "" || choice == "new" || choice == "create new" {
		cluster, err := createCluster(proj, create)
		if err != nil {
			return "", fmt.Errorf("create cluster in %s: %v", proj, err)
		}
		return cluster.context(), nil
	}

	cluster, ok := clusters[choice]
	if !ok {
		return "", fmt.Errorf("cluster not found: %s", choice)
	}
	cmd := exec.Command("gcloud", "container", "clusters", "get-credentials", cluster.name, "--project="+cluster.project, "--zone="+cluster.zone)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("get credentials: %v", err)
	}
	return cluster.context(), nil
}

// contextConfig returns the loader and config, which can create a clientconfig.
func contextConfig() (clientcmd.ClientConfigLoader, *clientcmdapi.Config, error) {
	if err := ensureKubectl(); err != nil {
		fmt.Println("Prow's tackler requires kubectl, please install:")
		fmt.Println("  *", err)
		if gerr := ensureGcloud(); gerr != nil {
			fmt.Println("  *", gerr)
		}
		return nil, nil, errors.New("missing kubectl")
	}

	l := clientcmd.NewDefaultClientConfigLoadingRules()
	c, err := l.Load()
	return l, c, err
}

// selectContext allows the user to choose a context
// This may involve creating a cluster
func selectContext(co contextOptions) (string, error) {
	fmt.Println("Existing kubernetes contexts:")
	// get cluster context
	_, cfg, err := contextConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load ~/.kube/config from any obvious location")
	}
	// list contexts and ask to user to choose a context
	options := map[int]string{}

	var ctxs []string
	for ctx := range cfg.Contexts {
		ctxs = append(ctxs, ctx)
	}
	sort.Strings(ctxs)
	for idx, ctx := range ctxs {
		options[idx] = ctx
		if ctx == cfg.CurrentContext {
			fmt.Printf("* %d: %s (current)", idx, ctx)
		} else {
			fmt.Printf("  %d: %s", idx, ctx)
		}
		fmt.Println()
	}
	fmt.Println()
	choice := co.context
	switch {
	case choice != "":
		fmt.Println("Reuse " + choice + " context...")
	case co.create != "" || co.reuse != "":
		choice = "create"
		fmt.Println("Create new context...")
	default:
		choice = prompt("Choose context or", "create new")
	}

	if choice == "create" || choice == "" || choice == "create new" || choice == "new" {
		ctx, err := createContext(co)
		if err != nil {
			return "", fmt.Errorf("create context: %v", err)
		}
		return ctx, nil
	}

	if _, ok := cfg.Contexts[choice]; ok {
		return choice, nil
	}

	idx, err := strconv.Atoi(choice)
	if err != nil {
		return "", fmt.Errorf("invalid context: %q", choice)
	}

	if ctx, ok := options[idx]; ok {
		return ctx, nil
	}

	return "", fmt.Errorf("invalid index: %d", idx)
}

// applyCreate will dry-run create and then pipe this to kubectl apply.
//
// If we use the create verb it will fail if the secret already exists.
// And kubectl will reject the apply verb with a secret.
func applyCreate(ctx string, args ...string) error {
	create := exec.Command("kubectl", append([]string{"--dry-run=true", "--output=yaml", "create"}, args...)...)
	create.Stderr = os.Stderr
	obj, err := create.StdoutPipe()
	if err != nil {
		return fmt.Errorf("rolebinding pipe: %v", err)
	}

	if err := create.Start(); err != nil {
		return fmt.Errorf("start create: %v", err)
	}
	if err := apply(ctx, obj); err != nil {
		return fmt.Errorf("apply: %v", err)
	}
	if err := create.Wait(); err != nil {
		return fmt.Errorf("create: %v", err)
	}
	return nil
}

func apply(ctx string, in io.Reader) error {
	apply := exec.Command("kubectl", "--context="+ctx, "apply", "-f", "-")
	apply.Stderr = os.Stderr
	apply.Stdout = os.Stdout
	apply.Stdin = in
	if err := apply.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	return apply.Wait()
}

func applyRoleBinding(context string) error {
	who, err := currentAccount()
	if err != nil {
		return fmt.Errorf("current account: %v", err)
	}
	return applyCreate(context, "clusterrolebinding", "prow-admin", "--clusterrole=cluster-admin", "--user="+who)
}

type options struct {
	githubTokenPath string
	starter         string
	repos           flagutil.Strings
	contextOptions
	confirm bool
}

type contextOptions struct {
	context string
	create  string
	reuse   string
	project string
}

func addFlags(fs *flag.FlagSet) *options {
	var o options
	fs.StringVar(&o.githubTokenPath, "github-token-path", "", "Path to github token")
	fs.StringVar(&o.starter, "starter", "", "Apply starter.yaml from the following path or URL (use upstream for latest)")
	fs.Var(&o.repos, "repo", "Send prow webhooks for these orgs or org/repos (repeat as necessary)")
	fs.StringVar(&o.context, "context", "", "Choose kubeconfig context to use")
	fs.StringVar(&o.create, "create", "", "name of cluster to create in --project")
	fs.StringVar(&o.reuse, "reuse", "", "Reuse existing cluster in --project")
	fs.StringVar(&o.project, "project", "", "GCP project to get/create cluster")
	fs.BoolVar(&o.confirm, "confirm", false, "Overwrite existing prow deployments without asking if set")
	return &o
}

func githubToken(choice string) (string, error) {
	if choice == "" {
		fmt.Print("Store your GitHub token in a file e.g. echo $TOKEN > /path/to/github/token\n")
		choice = prompt("Input /path/to/github/token to upload into cluster", "")
	}
	path := os.ExpandEnv(choice)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("open %s: %v", path, err)
	}
	return path, nil
}

func githubClient(tokenPath string, dry bool) (github.Client, error) {
	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{tokenPath}); err != nil {
		return nil, fmt.Errorf("start agent: %v", err)
	}

	gen := secretAgent.GetTokenGenerator(tokenPath)
	censor := secretAgent.Censor
	if dry {
		return github.NewDryRunClient(gen, censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint), nil
	}
	return github.NewClient(gen, censor, github.DefaultGraphQLEndpoint, github.DefaultAPIEndpoint), nil
}

func applySecret(ctx, ns, name, key, path string) error {
	return applyCreate(ctx, "secret", "generic", name, "--from-file="+key+"="+path, "--namespace="+ns)
}

func applyStarter(kc *kubernetes.Clientset, ns, choice, ctx string, overwrite bool) error {
	const defaultStarter = "https://raw.githubusercontent.com/kubernetes/test-infra/master/config/prow/cluster/starter-gcs.yaml"

	if choice == "" {
		choice = prompt("Apply starter.yaml from", "github upstream")
	}
	if choice == "" || choice == "github" || choice == "upstream" || choice == "github upstream" {
		choice = defaultStarter
		fmt.Println("Loading from", choice)
	}
	_, err := kc.AppsV1().Deployments(ns).Get(context2.TODO(), "plank", metav1.GetOptions{})
	switch {
	case err != nil && apierrors.IsNotFound(err):
		// Great, new clean namespace to deploy!
	case err != nil: // unexpected error
		return fmt.Errorf("get plank: %v", err)
	case !overwrite: // already a plank, confirm overwrite
		overwriteChoice := prompt(fmt.Sprintf("Prow is already deployed to %s in %s, overwrite?", ns, ctx), "no")
		switch overwriteChoice {
		case "y", "Y", "yes":
			// carry on, then
		default:
			return errors.New("prow already deployed")
		}
	}
	apply := exec.Command("kubectl", "--context="+ctx, "apply", "-f", choice)
	apply.Stderr = os.Stderr
	apply.Stdout = os.Stdout
	return apply.Run()
}

func clientConfigNamespace(context string) (string, bool, error) {
	loader, cfg, err := contextConfig()
	if err != nil {
		return "", false, fmt.Errorf("load contexts: %v", err)
	}

	return clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).Namespace()
}

func clientConfig(context string) (*rest.Config, error) {
	loader, cfg, err := contextConfig()
	if err != nil {
		return nil, fmt.Errorf("load contexts: %v", err)
	}

	return clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).ClientConfig()
}

func ingress(kc *kubernetes.Clientset, ns, service string) (url.URL, error) {
	for {
		var ing *networking.IngressList
		var err error

		// Detect ingress API to use based on Kubernetes version
		if hasResource(kc.Discovery(), networking.SchemeGroupVersion.WithResource("ingresses")) {
			ing, err = kc.NetworkingV1beta1().Ingresses(ns).List(context2.TODO(), metav1.ListOptions{})
		} else {
			oldIng, err := kc.ExtensionsV1beta1().Ingresses(ns).List(context2.TODO(), metav1.ListOptions{})
			if err == nil {
				ing, err = toNewIngress(oldIng)
			}
		}

		if err != nil {
			logrus.WithError(err).Fatalf("Could not get ingresses for service: %s", service)
		}

		var best url.URL
		points := 0
		for _, ing := range ing.Items {
			// does this ingress route to the hook service?
			cur := -1
			var maybe url.URL
			for _, r := range ing.Spec.Rules {
				h := r.IngressRuleValue.HTTP
				if h == nil {
					continue
				}
				for _, p := range h.Paths {
					if p.Backend.ServiceName != service {
						continue
					}
					maybe.Scheme = "http"
					maybe.Host = r.Host
					maybe.Path = p.Path
					cur = 0
					break
				}
			}
			if cur < 0 {
				continue // no
			}

			// does it have an ip or hostname?
			for _, tls := range ing.Spec.TLS {
				for _, h := range tls.Hosts {
					if h == maybe.Host {
						cur = 3
						maybe.Scheme = "https"
						break
					}
				}
			}

			if cur == 0 {
				for _, i := range ing.Status.LoadBalancer.Ingress {
					if maybe.Host != "" && maybe.Host == i.Hostname {
						cur = 2
						break
					}
					if i.IP != "" {
						cur = 1
						if maybe.Host == "" {
							maybe.Host = i.IP
						}
						break
					}
				}
			}
			if cur > points {
				best = maybe
				points = cur
			}
		}
		if points > 0 {
			return best, nil
		}
		fmt.Print(".")
		time.Sleep(1 * time.Second)
	}
}

func hmacSecret() string {
	buf := make([]byte, 20)
	rand.Read(buf)
	return fmt.Sprintf("%x", buf)
}

func findHook(client github.Client, org, repo string, loc url.URL) (*github.Hook, error) {
	loc.Scheme = ""
	goal := loc.String()
	var hooks []github.Hook
	var err error
	if repo == "" {
		hooks, err = client.ListOrgHooks(org)
	} else {
		hooks, err = client.ListRepoHooks(org, repo)
	}
	if err != nil {
		return nil, fmt.Errorf("list hooks: %v", err)
	}

	for _, h := range hooks {
		u, err := url.Parse(h.Config.URL)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid %s/%s hook url %s", org, repo, h.Config.URL)
			continue
		}
		u.Scheme = ""
		if u.String() == goal {
			return &h, nil
		}
	}
	return nil, nil
}

func orgRepo(in string) (string, string) {
	parts := strings.SplitN(in, "/", 2)
	org := parts[0]
	var repo string
	if len(parts) == 2 {
		repo = parts[1]
	}
	return org, repo
}

func ensureHmac(kc *kubernetes.Clientset, ns string) (string, error) {
	secret, err := kc.CoreV1().Secrets(ns).Get(context2.TODO(), "hmac-token", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("get: %v", err)
	}
	if err == nil {
		buf, ok := secret.Data["hmac"]
		if ok {
			return string(buf), nil
		}
		logrus.Warn("hmac-token secret does not contain an hmac key, replacing secret with new random data...")
	} else {
		logrus.Info("Creating new hmac-token secret with random data...")
	}
	hmac := hmacSecret()
	secret = &corev1.Secret{}
	secret.Name = "hmac-token"
	secret.Namespace = ns
	secret.StringData = map[string]string{"hmac": hmac}
	if err == nil {
		if _, err = kc.CoreV1().Secrets(ns).Update(context2.TODO(), secret, metav1.UpdateOptions{}); err != nil {
			return "", fmt.Errorf("update: %v", err)
		}
	} else {
		if _, err = kc.CoreV1().Secrets(ns).Create(context2.TODO(), secret, metav1.CreateOptions{}); err != nil {
			return "", fmt.Errorf("create: %v", err)
		}
	}
	return hmac, nil
}

func enableHooks(client github.Client, loc url.URL, secret string, repos ...string) ([]string, error) {
	var enabled []string
	locStr := loc.String()
	hasFlagValues := len(repos) > 0
	for {
		var choice string
		switch {
		case !hasFlagValues:
			if len(enabled) > 0 {
				fmt.Println("Enabled so far:", strings.Join(enabled, ", "))
			}
			choice = prompt("Enable which org or org/repo", "quit")
		case len(repos) > 0:
			choice = repos[0]
			repos = repos[1:]
		default:
			choice = ""
		}
		if choice == "" || choice == "quit" {
			return enabled, nil
		}
		org, repo := orgRepo(choice)
		hook, err := findHook(client, org, repo, loc)
		if err != nil {
			return enabled, fmt.Errorf("find %s hook in %s: %v", locStr, choice, err)
		}
		yes := true
		j := "json"
		req := github.HookRequest{
			Name:   "web",
			Active: &yes,
			Events: github.AllHookEvents,
			Config: &github.HookConfig{
				URL:         locStr,
				ContentType: &j,
				Secret:      &secret,
			},
		}
		if hook == nil {
			var id int
			if repo == "" {
				id, err = client.CreateOrgHook(org, req)
			} else {
				id, err = client.CreateRepoHook(org, repo, req)
			}
			if err != nil {
				return enabled, fmt.Errorf("create %s hook in %s: %v", locStr, choice, err)
			}
			fmt.Printf("Created hook %d to %s in %s", id, locStr, choice)
			fmt.Println()
		} else {
			if repo == "" {
				err = client.EditOrgHook(org, hook.ID, req)
			} else {
				err = client.EditRepoHook(org, repo, hook.ID, req)
			}
			if err != nil {
				return enabled, fmt.Errorf("edit %s hook %d in %s: %v", locStr, hook.ID, choice, err)
			}
		}
		enabled = append(enabled, choice)
	}
}

func ensureConfigMap(kc *kubernetes.Clientset, ns, name, key string) error {
	cm, err := kc.CoreV1().ConfigMaps(ns).Get(context2.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get: %v", err)
		}
		cm = &corev1.ConfigMap{
			Data: map[string]string{key: ""},
		}
		cm.Name = name
		cm.Namespace = ns
		_, err := kc.CoreV1().ConfigMaps(ns).Create(context2.TODO(), cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create: %v", err)
		}
	}

	if _, ok := cm.Data[key]; ok {
		return nil
	}
	logrus.Warnf("%s/%s missing key %s, adding...", ns, name, key)
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[key] = ""
	if _, err := kc.CoreV1().ConfigMaps(ns).Update(context2.TODO(), cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update: %v", err)
	}
	return nil
}

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	skipGitHub := fs.Bool("skip-github", false, "Do not add github webhooks if set")
	opt := addFlags(fs)
	fs.Parse(os.Args[1:])

	const ns = "default"

	ctx, err := selectContext(opt.contextOptions)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to select context")
	}

	ctxNamespace, _, err := clientConfigNamespace(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to reload ~/.kube/config from any obvious location")
	}

	if ctxNamespace != ns {
		logrus.Warnf("Context %s specifies namespace %s, but Prow resources will be installed in namespace %s.", ctx, ctxNamespace, ns)
	}

	// get kubernetes client
	clientCfg, err := clientConfig(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to reload ~/.kube/config from any obvious location")
	}

	kc, err := kubernetes.NewForConfig(clientCfg)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create kubernetes client")
	}

	fmt.Println("Applying admin role bindings (to create RBAC rules)...")
	if err := applyRoleBinding(ctx); err != nil {
		logrus.WithError(err).Fatalf("Failed to apply cluster role binding to %s", ctx)
	}

	// configure plugins.yaml and config.yaml
	// TODO(fejta): throw up an editor
	if err = ensureConfigMap(kc, ns, "config", "config.yaml"); err != nil {
		logrus.WithError(err).Fatal("Failed to ensure config.yaml exists")
	}
	if err = ensureConfigMap(kc, ns, "plugins", "plugins.yaml"); err != nil {
		logrus.WithError(err).Fatal("Failed to ensure plugins.yaml exists")
	}

	fmt.Println("Deploying prow...")
	if err := applyStarter(kc, ns, opt.starter, ctx, opt.confirm); err != nil {
		logrus.WithError(err).Fatal("Could not deploy prow")
	}

	if !*skipGitHub {
		fmt.Println("Checking github credentials...")
		// create github client
		token, err := githubToken(opt.githubTokenPath)
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get github token")
		}
		client, err := githubClient(token, false)
		if err != nil {
			logrus.WithError(err).Fatal("Failed to create github client")
		}
		who, err := client.BotName()
		if err != nil {
			logrus.WithError(err).Fatal("Cannot access github account name")
		}
		fmt.Println("Prow will act as", who, "on github")

		// create github secrets
		fmt.Print("Applying github token into oauth-token secret...")
		if err := applySecret(ctx, ns, "oauth-token", "oauth", token); err != nil {
			logrus.WithError(err).Fatal("Could not apply github oauth token secret")
		}

		fmt.Print("Ensuring hmac secret exists at hmac-token...")
		hmac, err := ensureHmac(kc, ns)
		if err != nil {
			logrus.WithError(err).Fatal("Failed to ensure hmac-token exists")
		}
		fmt.Println("exists")

		fmt.Print("Looking for prow's hook ingress URL... ")
		url, err := ingress(kc, ns, "hook")
		if err != nil {
			logrus.WithError(err).Fatal("Could not determine webhook ingress URL")
		}
		fmt.Println(url.String())

		// TODO(fejta): ensure plugins are enabled for all these repos
		_, err = enableHooks(client, url, hmac, opt.repos.Strings()...)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not configure repos to send %s webhooks.", url.String())
		}
	}

	deck, err := ingress(kc, ns, "deck")
	if err != nil {
		logrus.WithError(err).Fatalf("Could not find deck URL")
	}
	deck.Path = strings.TrimRight(deck.Path, "*")
	fmt.Printf("Enjoy your %s prow instance at: %s!", ctx, deck.String())
	fmt.Println()
}
