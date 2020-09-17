/*
Copyright 2016 The Kubernetes Authors.

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
	"io"
	"text/template"
)

const tmplPageHeaderText = `
    <!doctype html>
   	<html>
   	<head>
   	    <link rel="stylesheet" type="text/css" href="/styles/style.css">
   	    <meta charset="utf-8">
   	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
   	    <title>GCS browser: {{.Name}}</title>
		<style>
		header {
			margin-left: 10px;
		}

		.next-button {
			margin: 10px 0;
		}

		.grid-head {
			border-bottom: 1px solid black;
		}

		.resource-grid {
			margin-right: 20px;
		}

		li.grid-row:nth-child(even) {
			background-color: #ddd;
		}

		li div {
			box-sizing: border-box;
			border-left: 1px solid black;
			padding-left: 5px;
			overflow-wrap: break-word;
		}
		li div:first-child {
			border-left: none;
		}

		</style>
   	</head>
   	<body>
`

var tmplPageHeader = template.Must(template.New("page-header").Parse(tmplPageHeaderText))

func htmlPageHeader(out io.Writer, name string) error {
	args := struct {
		Name string
	}{
		Name: name,
	}
	return tmplPageHeader.Execute(out, args)
}

const tmplPageFooterText = `</body></html>`

var tmplPageFooter = template.Must(template.New("page-footer").Parse(tmplPageFooterText))

func htmlPageFooter(out io.Writer) error {
	return tmplPageFooter.Execute(out, struct{}{})
}

const tmplContentHeaderText = `
    <header>
        <h1>{{.DirName}}</h1>
        <h3>{{.Path}}</h3>
    </header>
    <ul class="resource-grid">
`

var tmplContentHeader = template.Must(template.New("content-header").Parse(tmplContentHeaderText))

func htmlContentHeader(out io.Writer, dirname, path string) error {
	args := struct {
		DirName string
		Path    string
	}{
		DirName: dirname,
		Path:    path,
	}
	return tmplContentHeader.Execute(out, args)
}

const tmplContentFooterText = `</ul>`

var tmplContentFooter = template.Must(template.New("content-footer").Parse(tmplContentFooterText))

func htmlContentFooter(out io.Writer) error {
	return tmplContentFooter.Execute(out, struct{}{})
}

const tmplNextButtonText = `
    <a href="{{.Path}}?marker={{.Marker}}"
	   class="pure-button next-button">
	   Next page
	</a>
`

var tmplNextButton = template.Must(template.New("next-button").Parse(tmplNextButtonText))

func htmlNextButton(out io.Writer, path, marker string) error {
	args := struct {
		Path   string
		Marker string
	}{
		Path:   path,
		Marker: marker,
	}
	return tmplNextButton.Execute(out, args)
}

const tmplGridHeaderText = `
	<li class="pure-g">
		<div class="pure-u-2-5 grid-head">Name</div>
		<div class="pure-u-1-5 grid-head">Size</div>
		<div class="pure-u-2-5 grid-head">Modified</div>
	</li>
`

var tmplGridHeader = template.Must(template.New("grid-header").Parse(tmplGridHeaderText))

func htmlGridHeader(out io.Writer) error {
	return tmplGridHeader.Execute(out, struct{}{})
}

const tmplGridItemText = `
    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="{{.URL}}"><img src="{{.Icon}}"> {{.Name}}</a></div>
	    <div class="pure-u-1-5">{{.Size}}</div>
	    <div class="pure-u-2-5">{{.Modified}}</div>
	</li>
`

var tmplGridItem = template.Must(template.New("grid-item").Parse(tmplGridItemText))

func htmlGridItem(out io.Writer, icon, url, name, size, modified string) error {
	args := struct {
		URL      string
		Icon     string
		Name     string
		Size     string
		Modified string
	}{
		URL:      url,
		Icon:     icon,
		Name:     name,
		Size:     size,
		Modified: modified,
	}
	return tmplGridItem.Execute(out, args)
}
