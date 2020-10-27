/*
Copyright 2020 The Kubernetes Authors.

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

// Package trickortreat adds halloween images to an issue or PR in response to a /trick-or-treat comment
package trickortreat

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match        = regexp.MustCompile(`(?mi)^/trick(\-)?or(\-)?treat(?: (.+))?\s*$`)
	trickortreat = &realClowder{}

	// A curated list that contains safe images URLs that we feel confident to share.
	// TODO: use an API for obtaining list of images, for example from https://www.pexels.com/api/documentation.
	candiesImgs = []string{
		"https://images.pexels.com/photos/1906435/pexels-photo-1906435.jpeg?cs=srgb&dl=fruit-candies-1906435.jpg&fm=jpg",
		"https://get.pxhere.com/photo/texture-food-produce-dessert-colors-background-candy-assorted-assortment-confectionery-candies-bright-colors-tic-tacs-lollies-fruit-adventure-gummi-candy-wine-gum-jelly-bean-615721.jpg",
		"https://static.pexels.com/photos/539447/pexels-photo-539447.jpeg",
		"https://get.pxhere.com/photo/assorted-assortment-background-bright-candies-candy-cane-chewy-close-up-color-colorful-confection-confectionery-delicious-dessert-edible-flavor-food-goodies-gummy-jelly-lollipop-motley-red-snack-sprinkles-stick-striped-sugar-sweet-sweets-tasty-texture-treats-twisted-variety-yummy-1558357.jpg",
		"https://get.pxhere.com/photo/background-birthday-bonbon-candy-candy-cane-candycane-cane-celebration-chewy-childhood-closeup-colorful-confection-confectionery-confetti-sprinkles-delicious-dessert-edible-festive-flavor-food-gummy-jelly-junk-lollipop-ornament-rainbow-red-snack-sprinkle-sprinkles-stick-striped-sugar-sweetmeat-sweets-taste-tasty-texture-treat-twisted-unhealthy-variety-yummy-sweetness-vegetarian-food-1433635.jpg",
		"https://c.pxhere.com/photos/4d/3b/sweets_jellies_sweet_candy_gum_candies-709927.jpg!s",
		"https://cdn.pixabay.com/photo/2015/04/23/02/21/candy-735595_960_720.jpg",
		"https://get.pxhere.com/photo/background-bakery-birthday-bonbon-button-candy-candy-background-candy-world-celebration-childhood-closeup-colorful-confection-confectionery-confetti-confetti-sprinkles-decoration-delicious-festive-food-goodies-group-junk-mix-ornament-rainbow-sprinkles-sugar-sugar-candy-sugary-sweet-texture-textured-topping-treat-wallpaper-sweetness-mixture-jelly-bean-nonpareils-1446217.jpg",
		"https://get.pxhere.com/photo/food-produce-color-market-colorful-dessert-toy-eyes-children-treats-sweets-candy-sweetness-confectionery-flavor-gummy-599314.jpg",
		"https://static.pexels.com/photos/35028/valentine-candy-hearts-conversation-sweet.jpg",
		"https://get.pxhere.com/photo/sweet-colors-candy-sweetness-sprinkles-confectionery-mixture-flavor-pixie-nonpareils-jelly-bean-1372644.jpg",
		"https://images.pexels.com/photos/1875919/pexels-photo-1875919.jpeg?cs=srgb&dl=colourful-candies-1875919.jpg&fm=jpg",
		"https://get.pxhere.com/photo/assorted-assortment-background-bonbon-calories-candy-candy-shop-candy-store-cc0-chewy-childhood-closeup-colorful-confection-confectionery-creative-common-0-creative-commons-0-delicious-dessert-diabetic-edible-excessive-flavor-food-gelatin-gummy-halloween-isolated-jelly-jelly-background-jelly-worm-junk-party-rainbow-shop-snack-store-sugar-sugar-jelly-sugary-sweet-sweet-tooth-sweets-taste-tasty-texture-textured-unhealthy-wallpaper-worms-yummy-sweetness-gummi-candy-fruit-taffy-wine-gum-local-food-vegetarian-food-1447325.jpg",
		"https://images.pexels.com/photos/136745/pexels-photo-136745.jpeg?cs=srgb&dl=beads-blur-bright-candy-136745.jpg&fm=jpg",
		"https://images.pexels.com/photos/90919/pexels-photo-90919.png?cs=srgb&dl=colorful-colourful-candy-90919.jpg&fm=jpg",
		"https://cdn.pixabay.com/photo/2013/07/25/11/51/jellybean-166828_960_720.jpg",
		"https://cdn.pixabay.com/photo/2014/04/05/11/30/sweets-316059_640.jpg",
		"https://c.pxhere.com/photos/99/57/candy_sweetmeats_sweets_caramel_dessert_food_colorful_bright-1376962.jpg!d",
		"https://static.pexels.com/photos/6152/sugar-candy-sweet-pink.jpg",
		"https://cdn.pixabay.com/photo/2015/05/10/01/41/starburst-760417_960_720.jpg",
		"https://get.pxhere.com/photo/plant-fruit-pattern-food-produce-dessert-colors-sweets-jellies-candy-sweetness-confectionery-candies-candied-fruit-gummi-candy-wine-gum-gumdrop-gelatin-dessert-1009431.jpg",
		"https://www.maxpixels.net/static/photo/640/Candy-Lollipops-Candies-Sugar-Colors-602441.jpg",
		"https://cdn.pixabay.com/photo/2013/08/10/18/41/candy-171343_960_720.jpg",
		"https://images.pexels.com/photos/443419/pexels-photo-443419.jpeg?auto=compress&cs=tinysrgb&h=750&w=1260",
		"https://cdn.pixabay.com/photo/2013/08/10/18/13/pick-and-mix-171342_640.jpg",
		"https://cdn.pixabay.com/photo/2015/09/25/01/11/candy-956555_960_720.jpg",
		"https://cdn.pixabay.com/photo/2015/03/27/00/14/chewy-candy-693888_960_720.jpg",
		"https://get.pxhere.com/photo/sweet-food-chocolate-dessert-toy-delicious-sugar-candy-diet-confectionery-candies-unhealthy-billiard-ball-jelly-bean-sixlets-1387119.jpgchristmas-candy-house-1477998087Nfb.jpg",
		"https://cdn.pixabay.com/photo/2015/03/27/00/09/chewy-candy-693867_640.jpg",
		"https://get.pxhere.com/photo/sweet-food-dessert-nuts-confectionery-chocolates-halloween-candy-snack-food-gift-basket-small-size-1061970.jpg",
		"https://c.pxhere.com/photos/75/02/chocolate_candies_sweets_snack_gourmet_box_treat_white-767846.jpg!d",
		"https://p0.pikrepo.com/preview/813/18/woman-s-lip-with-assorted-color-candies-thumbnail.jpg",
		"https://i1.pickpik.com/photos/139/673/305/candies-colorful-store-sweet-thumb.jpg",
		"https://i1.pickpik.com/photos/324/366/392/candies-colorful-store-sweet-preview.jpg",
		"https://get.pxhere.com/photo/sweet-food-dessert-delicious-sugar-candy-diet-confectionery-candies-unhealthy-sweetarts-1387223.jpg",
		"https://images.pexels.com/photos/1236662/pexels-photo-1236662.jpeg?cs=srgb&dl=close-up-photography-of-orange-candies-1236662.jpg&fm=jpg",
		"https://images.pexels.com/photos/618918/pexels-photo-618918.jpeg?cs=srgb&dl=blur-candies-close-up-confectionery-618918.jpg&fm=jpg",
		"https://images.pexels.com/photos/37537/cake-pops-candies-chocolate-food-37537.jpeg?auto=compress&cs=tinysrgb&fit=crop&h=627&w=1200",
		"https://get.pxhere.com/photo/sweet-food-produce-brown-dessert-caramel-delicious-fudge-sugar-candy-diet-confectionery-praline-unhealthy-bonbon-1386764.jpg",
		"https://c4.wallpaperflare.com/wallpaper/344/140/1013/chocolate-box-candies-allsorts-wallpaper-preview.jpg",
		"https://images.pexels.com/photos/3440657/pexels-photo-3440657.jpeg?cs=srgb&dl=photo-of-jar-with-candies-near-christmas-balls-3440657.jpg&fm=jpg",
		"https://images.pexels.com/photos/4114979/pexels-photo-4114979.jpeg?auto=compress&cs=tinysrgb&fit=crop&h=627&w=1200",
		"https://cdn.pixabay.com/photo/2017/07/01/22/30/cartoon-2462970_960_720.png",
		"https://img.rawpixel.com/s3fs-private/rawpixel_images/website_content/a019-jakubk-0900-sweet-candies-store3.jpg?auto=format&bg=transparent&con=3&cs=srgb&dpr=1&fm=jpg&ixlib=php-3.1.0&mark=rawpixel-watermark.png&markalpha=90&markpad=13&markscale=10&markx=25&q=75&usm=15&vib=3&w=1400&s=625c13af905d7c79413535c86b0249ff",
		"https://cdn.pixabay.com/photo/2013/07/12/11/59/candies-145068_640.png",
		"https://images.pexels.com/photos/4016594/pexels-photo-4016594.jpeg?auto=compress&cs=tinysrgb&h=750&w=1260",
		"https://i1.pickpik.com/photos/128/614/371/candies-colorful-store-sweet-thumb.jpg",
		"https://cdn.pixabay.com/photo/2016/07/13/09/43/gummy-bears-1514016_640.png",
		"https://images.pexels.com/photos/2064126/pexels-photo-2064126.jpeg?auto=compress&cs=tinysrgb&fit=crop&h=627&w=1200",
		"https://images.pexels.com/photos/2559743/pexels-photo-2559743.jpeg?cs=srgb&dl=awesome-candies-dessert-milkshake-2559743.jpg&fm=jpg",
		"https://images.pexels.com/photos/4114980/pexels-photo-4114980.jpeg?cs=srgb&dl=candies-and-flowers-on-black-surface-4114980.jpg&fm=jpg",
		"https://get.pxhere.com/photo/play-sweet-food-color-child-dessert-colour-children-background-bonbons-candy-sweetness-smarties-yummy-sprinkles-confectionery-lentils-snack-food-jelly-bean-1360826.jpg",
		"https://images.pexels.com/photos/413610/pexels-photo-413610.jpeg?cs=srgb&dl=candies-candy-candy-cane-child-413610.jpg&fm=jpg",
		"https://get.pxhere.com/photo/group-food-green-red-brown-colorful-yellow-snack-dessert-toy-sugar-bright-multicolored-candy-confection-skittles-assortment-confectionery-candies-jelly-bean-742963.jpg",
		"https://images.pexels.com/photos/1375807/pexels-photo-1375807.jpeg?auto=compress&cs=tinysrgb&h=750&w=1260",
		"https://images.pexels.com/photos/1578293/pexels-photo-1578293.jpeg?auto=compress&cs=tinysrgb&h=750&w=1260",
		"https://images.pexels.com/photos/307280/pexels-photo-307280.jpeg?cs=srgb&dl=cakes-candies-food-sweets-307280.jpg&fm=jpg",
		"https://images.pexels.com/photos/4015264/pexels-photo-4015264.jpeg?auto=compress&cs=tinysrgb&h=750&w=1260",
	}
)

const (
	pluginName = "trickortreat"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The trickortreat plugin adds a trickortreat image to an issue or PR in response to the `/trick-or-treat` command.",
		Config:      map[string]string{},
		Snippet:     "",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/trick(-)or(-)treat",
		Description: "Add a candy image to the issue or PR",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/trick-or-treat", "/trickortreat"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type clowder interface {
	readImage() (string, error)
}

type realClowder struct {
}

func (c *realClowder) readImage() (string, error) {
	var imgURL string
	var err error
	var toobig bool
	for i := 0; i < 3; i++ {
		imgIndex := rand.Intn(len(candiesImgs))
		imgURL := candiesImgs[imgIndex]
		// checking size, GitHub doesn't support big images
		toobig, err = github.ImageTooBig(imgURL)
		if err == nil && !toobig {
			return imgURL, nil
		}
	}
	if err == nil && toobig {
		err = fmt.Errorf("Image %q too big", imgURL)
	}
	return "", err
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(
		pc.GitHubClient,
		pc.Logger,
		&e,
		trickortreat,
	)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, c clowder) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a cat
	mat := match.FindStringSubmatch(e.Body)
	if mat == nil {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	for i := 0; i < 3; i++ {
		imgURL, err := c.readImage()
		if err != nil {
			log.WithError(err).Error("Failed to get img")
			continue
		}
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, imgURL))
	}

	return errors.New("could not find a valid candy image")
}
