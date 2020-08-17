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
	"github.com/gorilla/csrf"
	"github.com/sirupsen/logrus"
	"html/template"
	"k8s.io/test-infra/prow/cmd/deck/version"
	"k8s.io/test-infra/prow/config"
	"net/http"
	"path"
)

// This stuff is used in the templates.
type baseTemplateSettings struct {
	MobileFriendly bool
	DarkMode       bool
	PageName       string
	Arguments      interface{}
}

func makeBaseTemplateSettings(mobileFriendly bool, darkMode bool, pageName string, arguments interface{}) baseTemplateSettings {
	return baseTemplateSettings{mobileFriendly, darkMode, pageName, arguments}
}

func getConcreteBrandingFunction(cfg config.Getter) func() config.Branding {
	return func() config.Branding {
		if branding := cfg().Deck.Branding; branding != nil {
			return *branding
		}
		return config.Branding{}
	}
}

type baseTemplateSections struct {
	PR   bool
	Tide bool
}

func getConcreteSectionFunction(o options) func() baseTemplateSections {
	return func() baseTemplateSections {
		return baseTemplateSections{
			PR:   o.oauthURL != "" || o.pregeneratedData != "",
			Tide: o.tideURL != "" || o.pregeneratedData != "",
		}
	}
}

func prepareBaseTemplate(o options, cfg config.Getter, csrfToken string, t *template.Template) (*template.Template, error) {
	return t.Funcs(map[string]interface{}{
		"settings":         makeBaseTemplateSettings,
		"branding":         getConcreteBrandingFunction(cfg),
		"sections":         getConcreteSectionFunction(o),
		"mobileFriendly":   func() bool { return true },
		"mobileUnfriendly": func() bool { return false },
		"darkMode":         func() bool { return true },
		"lightMode":        func() bool { return false },
		"deckVersion":      func() string { return version.Version },
		"googleAnalytics":  func() string { return cfg().Deck.GoogleAnalytics },
		"csrfToken":        func() string { return csrfToken },
	}).ParseFiles(path.Join(o.templateFilesLocation, "base.html"))
}

func handleSimpleTemplate(o options, cfg config.Getter, templateName string, param interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := template.New(templateName) // the name matters, and must match the filename.
		if _, err := prepareBaseTemplate(o, cfg, csrf.Token(r), t); err != nil {
			logrus.WithError(err).Error("error preparing base template")
			http.Error(w, "error preparing base template", http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		if _, err := t.ParseFiles(path.Join(o.templateFilesLocation, templateName)); err != nil {
			logrus.WithError(err).Error("error parsing template " + templateName)
			http.Error(w, "error parsing template", http.StatusInternalServerError)
			return
		}

		if err := t.Execute(w, param); err != nil {
			logrus.WithError(err).Error("error executing template " + templateName)
			http.Error(w, "error executing template", http.StatusInternalServerError)
			return
		}
	}
}
