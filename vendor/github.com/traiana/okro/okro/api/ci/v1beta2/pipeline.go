package v1beta2

import (
	valid "github.com/go-ozzo/ozzo-validation"
	"github.com/traiana/okro/okro/pkg/util/validation"
)

type RepoPipelines struct {
	// URLs of repositories that should run the pipelines
	RepoURLs []string `json:"repo_urls"`
	// Pipelines
	Pipelines []*Pipeline `json:"pipelines"`
}

func (rp RepoPipelines) Validate() error {
	if err := valid.ValidateStruct(&rp,
		valid.Field(&rp.RepoURLs, valid.Required),
		valid.Field(&rp.Pipelines, valid.Required),
	); err != nil {
		return err
	}

	ddRepo := validation.Deduper{}
	for i, repo := range rp.RepoURLs {
		if dup := ddRepo.Add(repo, "repo_urls", i); dup != nil {
			return dup.AsNested("url")
		}
	}

	ddPipeline := validation.Deduper{}
	for i, pipe := range rp.Pipelines {
		if dup := ddPipeline.Add(pipe.Name, "pipelines", i); dup != nil {
			return dup.AsNested("pipeline")
		}
	}

	return nil
}

type Pipeline struct {
	// Pipeline name. Used as the context name on Github
	Name string `json:"name"`
	// Pipeline labels
	Labels map[string]string `json:"labels,omitempty"`

	// Do not run against these branches. Default is no branches
	SkipBranches []string `json:"skip_branches,omitempty"`
	// Only run against these branches. Default is all branches
	Branches []string `json:"branches,omitempty"`
	// Whether build failure prevents PR from merging
	Optional bool `json:"optional"`

	// Build steps
	Steps []Step `json:"steps"`
	// Build artifacts
	Artifacts []Artifact `json:"artifacts"`
}

func (p Pipeline) Validate() error {
	if err := valid.ValidateStruct(&p,
		valid.Field(&p.Name, valid.Required),
		valid.Field(&p.Steps, valid.Required),
		valid.Field(&p.Artifacts),
	); err != nil {
		return err
	}

	ddStep := validation.Deduper{}
	for i, step := range p.Steps {
		if dup := ddStep.Add(step.Name, "steps", i); dup != nil {
			return dup.AsNested("step")
		}
	}

	ddArtifact := validation.Deduper{}
	for i, art := range p.Artifacts {
		if dup := ddArtifact.Add(art.Name(), "artifacts", i); dup != nil {
			return dup.AsNested("artifact")
		}
	}

	return nil
}

type Step struct {
	Name   string   `json:"name"`
	Image  string   `json:"image,omitempty"`
	Script string   `json:"script"`
	Env    []EnvVar `json:"env,omitempty"`
}

func (s Step) Validate() error {
	return valid.ValidateStruct(&s,
		valid.Field(&s.Name, valid.Required),
		valid.Field(&s.Script, valid.Required),
		valid.Field(&s.Env),
	)
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (v EnvVar) Validate() error {
	return valid.ValidateStruct(&v,
		valid.Field(&v.Name, valid.Required),
		valid.Field(&v.Value, valid.Required),
	)
}
