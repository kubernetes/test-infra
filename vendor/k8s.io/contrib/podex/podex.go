/*
Copyright 2014 The Kubernetes Authors.

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

// podex is a command line tool to bootstrap kubernetes container
// manifest from docker image metadata.
//
// Manifests can then be edited by a human to match deployment needs.
//
// Example usage:
//
// $ docker pull google/nodejs-hello
// $ podex -format yaml google/nodejs-hello > google/nodejs-hello/pod.yaml
// $ podex -format json google/nodejs-hello > google/nodejs-hello/pod.json

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	goyaml "gopkg.in/yaml.v2"
)

const usage = "podex [-daemon] [-insecure-registry] [-insecure-skip-verify] [-format=yaml|json] [-type=pod|container] [-name NAME] IMAGES..."

var flManifestFormat = flag.String("format", "yaml", "manifest format to output, `yaml` or `json`")
var flManifestType = flag.String("type", "pod", "manifest type to output, `pod` or `container`")
var flManifestName = flag.String("name", "", "manifest name, default to image base name")
var flDaemon = flag.Bool("daemon", false, "daemon mode")
var flInsecureRegistry = flag.Bool("insecure-registry", false, "connect to insecure registry")
var flInsecureSkipVerify = flag.Bool("insecure-skip-verify", false, "skip certificate verify")

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s\n", usage)
		flag.PrintDefaults()
	}
}

type image struct {
	Host      string
	Namespace string
	Image     string
	Tag       string
}

func main() {
	flag.Parse()

	if *flDaemon {
		http.HandleFunc("/pods/", func(w http.ResponseWriter, r *http.Request) {
			image := strings.TrimPrefix(r.URL.Path, "/pods/")
			_, _, manifestName, _ := splitDockerImageName(image)
			manifest, err := getManifest(manifestName, "pod", "json", image)
			if err != nil {
				errMessage := fmt.Sprintf("failed to generate pod manifest for image %q: %v", image, err)
				log.Print(errMessage)
				http.Error(w, errMessage, http.StatusInternalServerError)
				return
			}
			io.Copy(w, manifest)
		})
		log.Fatal(http.ListenAndServe(":8080", nil))
	}

	if flag.NArg() < 1 {
		flag.Usage()
		log.Fatal("pod: missing image argument")
	}
	if *flManifestName == "" {
		if flag.NArg() > 1 {
			flag.Usage()
			log.Fatal("podex: -name arg is required when passing more than one image")
		}
		_, _, *flManifestName, _ = splitDockerImageName(flag.Arg(0))
	}
	if *flManifestType != "pod" && *flManifestType != "container" {
		flag.Usage()
		log.Fatalf("unsupported manifest type %q", *flManifestType)
	}
	if *flManifestFormat != "yaml" && *flManifestFormat != "json" {
		flag.Usage()
		log.Fatalf("unsupported manifest format %q", *flManifestFormat)
	}

	manifest, err := getManifest(*flManifestName, *flManifestType, *flManifestFormat, flag.Args()...)
	if err != nil {
		log.Fatalf("failed to generate %q manifest for %v: %v", *flManifestType, flag.Args(), err)
	}
	io.Copy(os.Stdout, manifest)
}

// getManifest infers a pod (or container) manifest for a list of docker images.
func getManifest(manifestName, manifestType, manifestFormat string, images ...string) (io.Reader, error) {
	podContainers := []goyaml.MapSlice{}

	for _, imageName := range images {
		host, namespace, repo, tag := splitDockerImageName(imageName)

		container := goyaml.MapSlice{
			{Key: "name", Value: repo},
			{Key: "image", Value: imageName},
		}

		img, err := getImageMetadata(host, namespace, repo, tag)

		if err != nil {
			return nil, fmt.Errorf("failed to get image metadata %q: %v", imageName, err)
		}
		portSlice := []goyaml.MapSlice{}
		for p := range img.ContainerConfig.ExposedPorts {
			port, err := strconv.Atoi(p.port())
			if err != nil {
				return nil, fmt.Errorf("failed to parse port %q: %v", p.port(), err)
			}
			portEntry := goyaml.MapSlice{{
				Key:   "name",
				Value: strings.Join([]string{p.proto(), p.port()}, "-"),
			}, {
				Key:   "containerPort",
				Value: port,
			}}
			portSlice = append(portSlice, portEntry)
			if p.proto() != "tcp" {
				portEntry = append(portEntry, goyaml.MapItem{Key: "protocol", Value: strings.ToUpper(p.proto())})
			}
		}
		if len(img.ContainerConfig.ExposedPorts) > 0 {
			container = append(container, goyaml.MapItem{Key: "ports", Value: portSlice})
		}
		podContainers = append(podContainers, container)
	}

	containerManifest := goyaml.MapSlice{
		{Key: "containers", Value: podContainers},
	}

	var data interface{}

	// TODO(proppy): add flag to handle multiple version
	apiVersion := goyaml.MapItem{Key: "apiVersion", Value: "v1"}

	switch manifestType {
	case "container":
		data = append(goyaml.MapSlice{apiVersion,
			{Key: "kind", Value: "ContainerList"},
			{Key: "metadata", Value: goyaml.MapSlice{}},
		}, containerManifest...)
	case "pod":
		data = goyaml.MapSlice{apiVersion,
			{Key: "kind", Value: "Pod"},
			{Key: "metadata", Value: goyaml.MapSlice{
				{Key: "name", Value: manifestName},
			}},
			{Key: "spec", Value: containerManifest},
		}
	default:
		return nil, fmt.Errorf("unsupported manifest type %q", manifestFormat)
	}

	yamlBytes, err := goyaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal container manifest: %v", err)
	}

	switch manifestFormat {
	case "yaml":
		return bytes.NewBuffer(yamlBytes), nil
	case "json":
		jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal container manifest into JSON: %v", err)
		}
		var jsonPretty bytes.Buffer
		if err := json.Indent(&jsonPretty, jsonBytes, "", "  "); err != nil {
			return nil, fmt.Errorf("failed to indent json %q: %v", string(jsonBytes), err)
		}
		return &jsonPretty, nil
	default:
		return nil, fmt.Errorf("unsupported manifest format %q", manifestFormat)
	}

}

// splitDockerImageName split a docker image name of the form [HOST/][NAMESPACE/]REPOSITORY[:TAG]
func splitDockerImageName(imageName string) (host, namespace, repo, tag string) {
	hostNamespaceImage := strings.Split(imageName, "/")
	last := len(hostNamespaceImage) - 1
	repoTag := strings.Split(hostNamespaceImage[last], ":")
	repo = repoTag[0]
	if len(repoTag) > 1 {
		tag = repoTag[1]
	}
	switch len(hostNamespaceImage) {
	case 2:
		host = ""
		namespace = hostNamespaceImage[0]
	case 3:
		host = hostNamespaceImage[0]
		namespace = hostNamespaceImage[1]
	}
	return
}

type port string

func (p port) port() string {
	parts := strings.Split(string(p), "/")
	return parts[0]
}

func (p port) proto() string {
	parts := strings.Split(string(p), "/")
	if len(parts) == 1 {
		return "tcp"
	}
	return parts[1]
}

type imageMetadata struct {
	ID              string `json:"id"`
	ContainerConfig struct {
		ExposedPorts map[port]struct{}
	} `json:"container_config"`
}

type imageManifest struct {
	History []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
}

func getImageMetadata(host, namespace, repo, tag string) (*imageMetadata, error) {
	scheme := "https"
	if *flInsecureRegistry {
		scheme = "http"
	}
	if namespace == "" {
		namespace = "library"
	}
	if tag == "" {
		tag = "latest"
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: *flInsecureSkipVerify,
			},
		},
	}

	if host == "" {
		return getImageMetadataV2(client, scheme, "registry-1.docker.io", namespace, repo, tag)
	}

	metadata, err := getImageMetadataV2(client, scheme, host, namespace, repo, tag)
	if err != nil {
		if metadata, err = getImageMetadataV1(client, scheme, host, namespace, repo, tag); err != nil {
			return nil, fmt.Errorf("can't get image metadata: %v", err)
		}
	}
	return metadata, err
}

type tokenData struct {
	Token string `json:"token"`
}

func getImageMetadataV2(client *http.Client, scheme, host, namespace, repo, tag string) (*imageMetadata, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s", scheme, host, namespace, repo, tag), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating manifest request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making manifest request to %q: %v", host, err)
	}

	if auth := resp.Header.Get("WWW-Authenticate"); resp.StatusCode == http.StatusUnauthorized && auth != "" {
		authParams := parseAuthHeader(auth)
		authReq, err := http.NewRequest("GET", fmt.Sprintf("%s?service=%s&scope=%s", authParams["realm"], authParams["service"], authParams["scope"]), nil)
		if err != nil {
			return nil, fmt.Errorf("error creating auth request: %v", err)
		}

		authResp, err := client.Do(authReq)
		if err != nil {
			return nil, fmt.Errorf("error making auth request to %q: %v", authParams["realm"], err)
		}

		var token tokenData
		if err := json.NewDecoder(authResp.Body).Decode(&token); err != nil {
			return nil, fmt.Errorf("error getting auth token: %v", err)
		}
		req.Header.Add("Authorization", "Bearer "+token.Token)
		resp, err = client.Do(req)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error getting image manifest: %v", resp.Status)
	}

	var manifest imageManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("error decoding image manifest: %v", err)
	}

	return decodeImageMetadata(strings.NewReader(manifest.History[0].V1Compatibility))
}

func parseAuthHeader(header string) (params map[string]string) {
	params = make(map[string]string)
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return
	}
	header = header[len(prefix):]
	for _, param := range strings.Split(header, ",") {
		if paramPair := strings.SplitN(param, "=", 2); len(paramPair) == 2 {
			params[paramPair[0]] = strings.Replace(paramPair[1], "\"", "", -1)
		}
	}
	return
}

func getImageMetadataV1(client *http.Client, scheme, host, namespace, repo, tag string) (*imageMetadata, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s://%s/v1/repositories/%s/%s/images", scheme, host, namespace, repo), nil)

	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Add("X-Docker-Token", "true")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to %q: %v", host, err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error getting X-Docker-Token from %s: %q", host, resp.Status)
	}

	endpoints := resp.Header.Get("X-Docker-Endpoints")
	token := resp.Header.Get("X-Docker-Token")
	req, err = http.NewRequest("GET", fmt.Sprintf("%s://%s/v1/repositories/%s/%s/tags/%s", scheme, endpoints, namespace, repo, tag), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Add("Authorization", "Token "+token)
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting image id for %s/%s:%s %v", namespace, repo, tag, err)
	}
	var imageID string
	if err = json.NewDecoder(resp.Body).Decode(&imageID); err != nil {
		return nil, fmt.Errorf("error decoding image id: %v", err)
	}
	req, err = http.NewRequest("GET", fmt.Sprintf("%s://%s/v1/images/%s/json", scheme, endpoints, imageID), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Add("Authorization", "Token "+token)
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting json for image %q: %v", imageID, err)
	}
	return decodeImageMetadata(resp.Body)
}

func decodeImageMetadata(r io.Reader) (*imageMetadata, error) {
	var image imageMetadata
	if err := json.NewDecoder(r).Decode(&image); err != nil {
		return nil, fmt.Errorf("error decoding image metadata: %v", err)
	}
	return &image, nil
}
