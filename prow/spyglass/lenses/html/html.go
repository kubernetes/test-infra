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

package html

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/net/html"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

func init() {
	lenses.RegisterLens(Lens{})
}

type Lens struct{}

type document struct {
	Filename string
	Title    string
	Content  string
}

// Config returns the lens's configuration.
func (lens Lens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Name:      "html",
		Title:     "HTML",
		Priority:  3,
		HideTitle: true,
	}
}

// Header renders the content of <head> from template.html.
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	t, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("<!-- FAILED LOADING HEADER: %v -->", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "header", nil); err != nil {
		return fmt.Sprintf("<!-- FAILED EXECUTING HEADER TEMPLATE: %v -->", err)
	}
	return buf.String()
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

// Body renders the <body>
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	if len(artifacts) == 0 {
		logrus.Error("html Body() called with no artifacts, which should never happen.")
		return "Why am I here? There is no html file"
	}

	documents := make([]document, 0)
	for _, artifact := range artifacts {
		content, err := artifact.ReadAll()
		if err != nil {
			logrus.WithError(err).WithField("artifact_url", artifact.CanonicalLink()).Warn("failed to read content")
			continue
		}
		name := filepath.Base(artifact.CanonicalLink())
		documents = append(documents, extractDocumentDetails(name, content))
	}

	template, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		logrus.WithError(err).Error("Error executing template.")
		return fmt.Sprintf("Failed to load template file: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := template.ExecuteTemplate(buf, "body", documents); err != nil {
		return fmt.Sprintf("failed to execute template: %v", err)
	}
	return buf.String()
}

// extractDocumentDetails parses the HTML to extract the title and
// meta description tag, if present.
func extractDocumentDetails(name string, content []byte) document {
	doc := document{
		Filename: name,
		Title:    name,
		Content:  string(content),
	}

	description := ""
	token := html.NewTokenizer(bytes.NewReader(content))
	isTitle := false
	for {
		switch token.Next() {
		case html.ErrorToken:
			doc.Content = injectHeightNotifier(doc.Content, name)
			// Escape double quotes as we are going to put this into an iframes srcdoc attribute. We can not reference the
			// src directly because we have to inject the height notifier.
			// Ref: https://html.spec.whatwg.org/multipage/iframe-embed-object.html#attr-iframe-srcdoc
			doc.Content = strings.ReplaceAll(doc.Content, `"`, `&quot;`)

			if description != "" {
				doc.Title = doc.Title + fmt.Sprintf(` <abbr class="icon material-icons" title="%s">info</abbr>`, description)
			}

			return doc
		case html.StartTagToken, html.SelfClosingTagToken:
			tt := token.Token()
			switch tt.Data {
			case "title":
				isTitle = true
			case "meta":
				content := ""
				isDescription := false
				for _, attr := range tt.Attr {
					if attr.Key == "name" && attr.Val == "description" {
						isDescription = true
					} else if attr.Key == "content" {
						content = attr.Val
					}
				}
				if isDescription {
					description = content
				}
			}
		case html.TextToken:
			if isTitle {
				isTitle = false
				tt := token.Token()
				if tt.Data != "" {
					doc.Title = tt.Data
				}
			}
		}
	}
}

// injectHeightNotifier injects a small javascript snippet that will tell the iframe container about the height
// of the iframe. Iframe height can only be set as an absolute value and CORS doesn't allow the container to
// query the iframe.
func injectHeightNotifier(content string, name string) string {
	return `<div id="wrapper">` + content + fmt.Sprintf(`</div><script type="text/javascript">
window.addEventListener("load", function(){
    if(window.self === window.top) return; // if w.self === w.top, we are not in an iframe
    send_height_to_parent_function = function(){
        var height = document.getElementById("wrapper").offsetHeight;
        parent.postMessage({"height" : height , "id": "%s"}, "*");
    }
    send_height_to_parent_function(); //whenever the page is loaded
    window.addEventListener("resize", send_height_to_parent_function); // whenever the page is resized
    var observer = new MutationObserver(send_height_to_parent_function);           // whenever DOM changes PT1
    var config = { attributes: true, childList: true, characterData: true, subtree:true}; // PT2
    observer.observe(window.document, config);                                            // PT3
});
</script>`, name)
}
