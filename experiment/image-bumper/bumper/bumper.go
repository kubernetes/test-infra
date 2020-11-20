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

package bumper

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	imageRegexp = regexp.MustCompile(`\b((?:[a-z0-9]+\.)?gcr\.io)/([a-z][a-z0-9-]{5,29}/[a-zA-Z0-9][a-zA-Z0-9_./-]+):([a-zA-Z0-9_.-]+)\b`)
	tagRegexp   = regexp.MustCompile(`(v?\d{8}-(?:v\d(?:[.-]\d+)*-g)?[0-9a-f]{6,10}|latest)(-.+)?`)
)

const (
	imageHostPart  = 1
	imageImagePart = 2
	imageTagPart   = 3
	tagVersionPart = 1
	tagExtraPart   = 2
)

type Client struct {
	tagCache   map[string]string
	httpClient http.Client
}

func NewClient() *Client {
	return &Client{
		tagCache:   map[string]string{},
		httpClient: http.Client{Timeout: 1 * time.Minute},
	}
}

type manifest map[string]struct {
	TimeCreatedMs string   `json:"timeCreatedMs"`
	Tags          []string `json:"tag"`
}

func DeconstructTag(tag string) (date, commit, variant string) {
	currentTagParts := tagRegexp.FindStringSubmatch(tag)
	if currentTagParts == nil {
		return "", "", ""
	}
	parts := strings.Split(currentTagParts[tagVersionPart], "-")
	return parts[0][1:], parts[len(parts)-1], currentTagParts[tagExtraPart]
}

// FindLatestTag returns the latest valid tag for the given image.
func (cli *Client) FindLatestTag(imageHost, imageName, currentTag string) (string, error) {
	k := imageHost + "/" + imageName + ":" + currentTag
	if result, ok := cli.tagCache[k]; ok {
		return result, nil
	}

	currentTagParts := tagRegexp.FindStringSubmatch(currentTag)
	if currentTagParts == nil {
		return "", fmt.Errorf("couldn't figure out the current tag in %q", currentTag)
	}
	if currentTagParts[tagVersionPart] == "latest" {
		return currentTag, nil
	}

	resp, err := cli.httpClient.Get("https://" + imageHost + "/v2/" + imageName + "/tags/list")
	if err != nil {
		return "", fmt.Errorf("couldn't fetch tag list: %v", err)
	}
	defer resp.Body.Close()

	result := struct {
		Manifest manifest `json:"manifest"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("couldn't parse tag information from registry: %v", err)
	}

	latestTag, err := pickBestTag(currentTagParts, result.Manifest)
	if err != nil {
		return "", err
	}

	cli.tagCache[k] = latestTag

	return latestTag, nil
}

func pickBestTag(currentTagParts []string, manifest manifest) (string, error) {
	// The approach is to find the most recently created image that has the same suffix as the
	// current tag. However, if we find one called "latest" (with appropriate suffix), we assume
	// that's the latest regardless of when it was created.
	var latestTime int64
	latestTag := ""
	for _, v := range manifest {
		bestVariant := ""
		override := false
		for _, t := range v.Tags {
			parts := tagRegexp.FindStringSubmatch(t)
			if parts == nil {
				continue
			}
			if parts[tagExtraPart] != currentTagParts[tagExtraPart] {
				continue
			}
			if parts[tagVersionPart] == "latest" {
				override = true
				continue
			}
			if bestVariant == "" || len(t) < len(bestVariant) {
				bestVariant = t
			}
		}
		if bestVariant == "" {
			continue
		}
		t, err := strconv.ParseInt(v.TimeCreatedMs, 10, 64)
		if err != nil {
			return "", fmt.Errorf("couldn't parse timestamp %q: %v", v.TimeCreatedMs, err)
		}
		if override || t > latestTime {
			latestTime = t
			latestTag = bestVariant
			if override {
				break
			}
		}
	}

	if latestTag == "" {
		return "", fmt.Errorf("failed to find a good tag")
	}

	return latestTag, nil
}

func updateAllTags(tagPicker func(host, image, tag string) (string, error), content []byte, imageFilter *regexp.Regexp) []byte {
	indexes := imageRegexp.FindAllSubmatchIndex(content, -1)
	// Not finding any images is not an error.
	if indexes == nil {
		return content
	}

	newContent := make([]byte, 0, len(content))
	lastIndex := 0
	for _, m := range indexes {
		newContent = append(newContent, content[lastIndex:m[imageTagPart*2]]...)
		host := string(content[m[imageHostPart*2]:m[imageHostPart*2+1]])
		image := string(content[m[imageImagePart*2]:m[imageImagePart*2+1]])
		tag := string(content[m[imageTagPart*2]:m[imageTagPart*2+1]])
		lastIndex = m[1]

		if tag == "" || (imageFilter != nil && !imageFilter.MatchString(host+"/"+image+":"+tag)) {
			newContent = append(newContent, content[m[imageTagPart*2]:m[1]]...)
			continue
		}

		latest, err := tagPicker(host, image, tag)
		if err != nil {
			log.Printf("Failed to update %s/%s:%s: %v.\n", host, image, tag, err)
			newContent = append(newContent, content[m[imageTagPart*2]:m[1]]...)
			continue
		}
		newContent = append(newContent, []byte(latest)...)
	}
	newContent = append(newContent, content[lastIndex:]...)

	return newContent
}

// UpdateFile updates a file in place.
func (cli *Client) UpdateFile(tagPicker func(imageHost, imageName, currentTag string) (string, error),
	path string, imageFilter *regexp.Regexp) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", path, err)
	}

	newContent := updateAllTags(tagPicker, content, imageFilter)

	if err := ioutil.WriteFile(path, newContent, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %v", path, err)
	}
	return nil
}

// GetReplacements returns the tag replacements that have been made.
func (cli *Client) GetReplacements() map[string]string {
	return cli.tagCache
}
