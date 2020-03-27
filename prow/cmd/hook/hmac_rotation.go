package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	add_hook "k8s.io/test-infra/experiment/add-hook"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"sigs.k8s.io/yaml"
	"sort"
	"strings"
	"time"
)

//func main() {
//
//}

const expiryDate = "9999-10-02T15:00:00Z"
var kc kubernetes.Interface
var ns string

func main() {

	k8ClientOption := flagutil.KubernetesClientOptions  {MasterURL:"",KubeConfig:"",

	}
	var err error

	kc, err = k8ClientOption.KubeClient()
	ns = "somerandomnamespace"
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create kubernetes client")
	}
	currentConfigYaml, err := getCurrentClusterSecret()
	if err!=nil{
		// something
	}

	currentConfig := map[string]github.HmacsForRepo{}

	if err := yaml.Unmarshal(currentConfigYaml, &currentConfig); err != nil {
		logrus.WithError(err).Info("couldn't unmarshal the hmac secret as hierarchical file")
		// Exit.
	}
	newConfig := config.ManagedWebhooks{}// TODO parse this information
	handleConfigUpdate(newConfig, currentConfig)
}

func handleConfigUpdate(newConfig config.ManagedWebhooks, currentConfig map[string]github.HmacsForRepo) {

	repoAdded := map[string]config.ManagedWebhookInfo{}
	repoRemoved := map[string]bool{}
	repoRotated := map[string]config.ManagedWebhookInfo{}

	for key, value := range newConfig {
		var repoName = key
		if _, ok := currentConfig[repoName]; ok {
			repoRotated[repoName] = value
		} else {
			repoAdded[repoName] = value
		}
	}

	for key := range currentConfig {
		var repoName = key
		if _, ok := newConfig[repoName]; !ok {
			repoRemoved[repoName] = true
		}
	}

	handleRemovedRepo(repoRemoved, currentConfig)
	handleAddedRepo(repoAdded, currentConfig)
	handledRotatedRepo(repoRotated, currentConfig)
}

func handledRotatedRepo(rotated map[string]config.ManagedWebhookInfo, currentConfig map[string]github.HmacsForRepo) {
	// for each rotated repo, we only onboard a new token when none of the existing token were created after user specified time.
	for repo, value := range rotated{
		needRotation:=true
		tokenCreatedAfter := value.TokenCreatedAfter

		for _, currentTokens := range currentConfig[repo]{
			if currentTokens.CreatedOn.After(tokenCreatedAfter){
				needRotation = false
				break
			}
		}
		if needRotation{
			onboardNewTokenForSingleRepo(repo, currentConfig)
		}
	}
}

func handleAddedRepo(added map[string]config.ManagedWebhookInfo, currentConfig map[string]github.HmacsForRepo) {
	for repo := range added {
		onboardNewTokenForSingleRepo(repo, currentConfig)
	}
}

// handleRemoveRepo handles webhook removal and hmac token removal from k8s cluster from all repos removed from declarative config.
func handleRemovedRepo(removed map[string]bool, currentConfig map[string]github.HmacsForRepo) {
	fs := flag.NewFlagSet("help test", flag.ExitOnError)
	for repo, _ := range removed {
		args := []string{"--repo=" + repo,
			"--delete-webhook=true",
		}
		if err := add_hook.HandleWebhookConfigChange(fs, args); err != nil {
			// Log and skip to next one.
			continue
		}
		delete(currentConfig, repo)
		if err:=commitConfig(currentConfig); err!=nil{
			// Log error?
		}
	}
}

func onboardNewTokenForSingleRepo(repo string, currentConfig map[string]github.HmacsForRepo) error{
	generatedToken, err := generateNewHmacToken()
	if err!=nil{
		return nil
	}

	updatedTokenList := github.HmacsForRepo{}
	orgName := strings.Split(repo, "/")[0]
	if val, ok := currentConfig[repo]; ok {
		// copy over all exisiting tokens for that repo.
		updatedTokenList= append(updatedTokenList, val...)
	} else if val, ok := currentConfig[orgName]; ok {
		// Current webhook is using org lvl token. So we need to promote that token to repo level as well.
		updatedTokenList = append(updatedTokenList, val...)
	} else {
		// Current webhook is using global token so we need to promote that token to repo level as well.
		// And then add a new token to this.
		globalTokens := currentConfig["*"]
		updatedTokenList = append(updatedTokenList, globalTokens...)
	}

	updatedTokenList = append(updatedTokenList, github.HmacSecret{Value: generatedToken, CreatedOn: time.Now(), Expiry: time.Now().Add(time.Hour * 100000)})
	currentConfig[repo] = updatedTokenList
	// Commit this config change so calls can now be authenticated using both old and new token.
	if err := commitConfig(currentConfig); err != nil {

	}

	// Update the github webhook to use new token.
	fs := flag.NewFlagSet("help test", flag.ExitOnError)

	args := []string{"--repo=" + repo,
		"--delete-webhook=true",
		"--hook-url=http://an.ip.addr.ess/hook",
		"--hmac-value=" + generatedToken,
		"--confirm=false",
	}
	if err := add_hook.HandleWebhookConfigChange(fs, args); err != nil {

	}

	// Remove old token from current config.
	pruneOldTokens(currentConfig[repo])

	// Commit this config change so calls using old token would start getting rejected.
	if err := commitConfig(currentConfig); err != nil {

	}
	return nil
}



func addNewToken(existingTokens github.HmacsForRepo) {
	//TODO change expiry time to something cleaner.
	if token, err := generateNewHmacToken(); err != nil {
		existingTokens = append(existingTokens, github.HmacSecret{Value: token, CreatedOn: time.Now(), Expiry: time.Now().Add(time.Hour * 100000)})
	}
}


// commitConfig saves given in-memory config to secret file used by prow cluster.
func commitConfig(currentConfig map[string]github.HmacsForRepo) error {
	secretContent, err := yaml.Marshal(&currentConfig)
	if err!=nil{
		// throw error
	}
	secret := &corev1.Secret{}
	secret.Name = "hmac-token"
	secret.Namespace = ns
	secret.StringData = map[string]string{"hmac": string(secretContent)}
	if _, err = kc.CoreV1().Secrets(ns).Update(secret); err != nil {
		// throw error
	}
	return nil
}

// pruneOldTokens removes all but most recent token from token config.
func pruneOldTokens(currentConfig github.HmacsForRepo) {

	sort.SliceStable(currentConfig, func(i, j int) bool {
		return currentConfig[i].CreatedOn.After(currentConfig[j].CreatedOn)
	})
	currentConfig = currentConfig[:1]
}

// generateNewHmacToken generates a hex encoded crypto random string of length 20.
func generateNewHmacToken() (string, error) {
	bytes := make([]byte, 20) // our hmac token are of length 20
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// getCurrentClusterSecret returns the hmac tokens currently configured in the cluster.
func getCurrentClusterSecret() ([]byte, error) {
	secret, err := kc.CoreV1().Secrets(ns).Get("hmac-token", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get: %v", err)
	}
	if err == nil {
		buf, ok := secret.Data["hmac"]
		if ok {
			return buf, nil
		}
	}
	return nil, fmt.Errorf("get: %v", err)
}