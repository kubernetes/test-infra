package main

import (
	"bytes"
	"context"
	"flag"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/tide"
)

var (
	prowURL = flag.String("prow-url", "https://deck-ci.svc.ci.openshift.org", "Prow frontend URL.")
	dryRun  = flag.Bool("dry-run", true, "Whether to mutate any real-world state.")

	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
)

func main() {
	flag.Parse()
	log := logrus.WithField("bin", "search")

	if *prowURL == "" {
		log.Fatalf("need a prow URL")
	}

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.WithError(err).Fatalf("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		log.WithError(err).Fatalf("Must specify a valid --github-endpoint URL.")
	}

	var ghc *github.Client
	if *dryRun {
		ghc = github.NewDryRunClient(oauthSecret, *githubEndpoint)
	} else {
		ghc = github.NewClient(oauthSecret, *githubEndpoint)
	}
	ghc.Throttle(800, 20)

	// TODO: Retries
	resp, err := http.Get(*prowURL + "/config")
	if err != nil {
		log.Fatalf("cannot get prow config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Fatalf("status code not 2XX: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("cannot read request body: %v", err)
	}

	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("cannot unmarshal data from prow: %v", err)
	}

	ctx := context.Background()

	for _, q := range cfg.Tide.Queries {
		poolPRs, err := tide.Search(ghc, log, ctx, q.Query())
		if err != nil {
			log.Errorf("%v", err)
			continue
		}
		log.Infof("Got PRs: %d", len(poolPRs))
	}
}
