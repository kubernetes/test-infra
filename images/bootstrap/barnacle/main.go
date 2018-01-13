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
   Implements cleaning up unwanted docker data root contents.
   This is similar to `docker system prune` but tuned to our needs.
   We use it to allow persisting base images while cleaning up everything else.
*/

package main

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var (
	// images newer than this will be deleted
	deleteYoungerThanDuration = time.Hour * 24 * 7
)

func removeAllContainers(cli *client.Client) {
	// list all containers
	listOptions := types.ContainerListOptions{
		Quiet: true,
		All:   true,
	}
	containers, err := cli.ContainerList(context.Background(), listOptions)
	if err != nil {
		log.Printf("Failed to list containers: %v\n", err)
		return
	}

	// reverse sort by Creation time so we delete newest containers first
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Created > containers[j].Created
	})

	// stop then remove (which implicitly kills) each container
	duration := time.Second * 1
	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}
	for _, container := range containers {
		log.Printf("Stopping container: %v %s with ID: %s\n",
			container.Names, container.Image, container.ID)
		err = cli.ContainerStop(context.Background(), container.ID, &duration)
		if err != nil {
			log.Printf("Error stopping container: %v\n", err)
		}

		log.Printf("Removing container: %v %s with ID: %s\n",
			container.Names, container.Image, container.ID)
		err = cli.ContainerRemove(context.Background(), container.ID, removeOptions)
		if err != nil {
			log.Printf("Error removing container: %v\n", err)
		}
	}
}

func removeImages(cli *client.Client, images []types.ImageSummary) {
	// reverse sort by Creation time so we delete newest images first
	sort.Slice(images, func(i, j int) bool {
		return images[i].Created > images[j].Created
	})
	// remove each image
	removeOptions := types.ImageRemoveOptions{
		Force:         true,
		PruneChildren: true,
	}
	for _, image := range images {
		log.Printf("Deleting image: %v with ID: %s and size %d\n",
			image.RepoTags, image.ID, image.Size)
		_, err := cli.ImageRemove(context.Background(), image.ID, removeOptions)
		if err != nil {
			log.Printf("Failed to delete image. %v\n", err)
		}
	}
}

func removeDanglingImages(cli *client.Client) {
	args := filters.NewArgs()
	args.Add("dangling", "true")
	images, err := cli.ImageList(context.Background(), types.ImageListOptions{
		All:     false,
		Filters: args,
	})
	if err != nil {
		log.Printf("Failed to list images: %v", err)
		return
	}
	removeImages(cli, images)
}

func pruneVolumes(cli *client.Client) {
	report, err := cli.VolumesPrune(context.Background(), filters.NewArgs())
	if err != nil {
		log.Printf("Failed to prune volumes: %v\n", err)
		return
	}
	marshalled, err := json.Marshal(report)
	if err != nil {
		log.Printf("Failed to marshal report: %v", err)
		return
	}
	log.Printf("Volume Prune results: %s\n", marshalled)
}

func removeRecentImages(cli *client.Client, minimumAge time.Duration) {
	// get images
	images, err := cli.ImageList(context.Background(), types.ImageListOptions{
		All: false,
	})
	if err != nil {
		log.Printf("Failed to list images: %v", err)
		return
	}

	// select images to delete
	now := time.Now()
	deleteImages := []types.ImageSummary{}
	for _, image := range images {
		age := now.Sub(time.Unix(image.Created, 0))
		// delete recently created images
		if age <= deleteYoungerThanDuration {
			deleteImages = append(deleteImages, image)
		}
	}

	// delete them
	removeImages(cli, deleteImages)
}

func main() {
	log.SetPrefix("[Barnacle] ")
	log.Println("Cleaning up Docker data root...")

	// create docker client
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v\n", err)
	}

	// make sure all containers are removed before removing images
	log.Println("Removing all containers.")
	removeAllContainers(cli)

	// remove containers young enough to not likely be pulled base images
	log.Println("Removing recently created images.")
	removeRecentImages(cli, deleteYoungerThanDuration)

	// delete remaining dangling images
	log.Println("Pruning dangling images.")
	removeDanglingImages(cli)

	// prune volumes
	log.Println("Pruning volumes.")
	pruneVolumes(cli)

	// all done!
	log.Println("Done cleaning up Docker data root.")
}
