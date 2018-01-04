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

/*
   Implements cleaning up unwanted docker data root.
   This is similar to `docker system prune` but tuned to our needs.
   TODO(bentheelder): rename this. prune-docker <-> docker prune is a bit lame.
*/

package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"
)

var (
	// images newer than this will be deleted
	deleteYoungerThanDuration = time.Hour * 24 * 7
)

// TODO(bentheelder): consider using official docker client library
// Image represents docker image metadata
type Image struct {
	ID         string
	Repository string
	Tag        string
	CreatedAt  time.Time
}

const (
	// Docker appears to just use .String() on the timestamps for these
	// https://golang.org/pkg/time/#Time.String
	createdAtTimestampFormat = "2006-01-02 15:04:05.999999999 -0700 MST"
)

func getImages(all bool) ([]Image, error) {
	args := []string{
		"images", "--format",
		// this should contain all fields in Image, comma seperated
		"{{.ID}},{{.Repository}},{{.Tag}},{{.CreatedAt}}",
	}
	if all {
		args = append(args, "--all")
	}
	b, err := exec.Command("docker", args...).Output()
	if err != nil {
		return nil, err
	}
	images := []Image{}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 4 {
			return nil, fmt.Errorf("failed to parse line: '%v'", line)
		}
		parsedTime, err := time.Parse(createdAtTimestampFormat, parts[3])
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp '%s' %v", parts[1], err)
		}
		images = append(images, Image{
			parts[0],
			parts[1],
			parts[2],
			parsedTime,
		})
	}
	return images, nil
}

func deleteImage(image Image) error {
	return exec.Command("docker", "rmi", "-f", image.ID).Run()
}

func runInShell(command string) error {
	output, err := exec.Command("/bin/bash", "-c", command).CombinedOutput()
	log.Printf("%s", output)
	return err
}

func removeAllContainers() {
	runInShell("docker ps -aq | xargs -r docker stop")
	runInShell("docker ps -aq | xargs -r docker rm")
}

func removeDanglingImages() {
	runInShell("docker images --filter dangling=true -qa | xargs -r docker rmi -f")
}

func pruneVolumes() {
	runInShell("docker volume prune -f")
}

func main() {
	log.Println("Cleaning up Docker data root...")
	now := time.Now()
	log.Println("Making sure all containers are stopped / removed.")
	removeAllContainers()
	log.Println("Getting images.")
	// TODO(bentheelder): make sure any lingering containers are removed from the data root
	// get image refs with metadata
	images, err := getImages(false)
	if err != nil {
		log.Fatalf("Failed to get image data, error: %v", err)
	}
	// find images to delete
	deleteImages := []Image{}
	for _, image := range images {
		age := now.Sub(image.CreatedAt)
		// delete recently created images
		if age <= deleteYoungerThanDuration ||
			// *definitely* delete kube-build images
			strings.Contains(image.Repository, "kube-build") {
			deleteImages = append(deleteImages, image)
		}
	}
	// reverse sort by CreatedAt so we delete newest images first
	sort.Slice(deleteImages, func(i, j int) bool {
		return deleteImages[i].CreatedAt.After(deleteImages[j].CreatedAt)
	})
	// delete images
	for _, image := range deleteImages {
		log.Printf("Deleting image: %s:%s with ID: %s\n",
			image.Repository, image.Tag, image.ID)
		err = deleteImage(image)
		if err != nil {
			log.Printf("Failed to delete image. %v\n", err)
		}
	}
	log.Println("Deleting dangling images.")
	// delete dangling images and volumes
	removeDanglingImages()
	log.Println("Pruning volumes.")
	log.Println("NOTE: The total reclaimed space below is ONLY for volumes.")
	pruneVolumes()
	log.Println("Done cleaning up Docker data root.")
}
