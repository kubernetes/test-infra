package v1beta2

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	valid "github.com/go-ozzo/ozzo-validation"
)

const (
	TypeDocker = "docker"
	TypeFile   = "file"
	TypeMaven   = "maven"
	TypeJar = "jar"
)

type Artifact struct {
	File   *File   `json:"file"`
	Docker *Docker `json:"docker"`
	Jar    *Jar    `json:"jar"`
	Maven  *Maven  `json:"maven"`
}

type artifactBase struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
}

type File struct {
	artifactBase `json:",inline"`
	SourcePath   string `json:"sourcepath"`
	TargetRepo   string `json:"targetrepo"`
	TargetPath   string `json:"targetpath"`
}

type Docker struct {
	artifactBase `json:",inline"`
	Dockerfile   string `json:"dockerfile"`
	Context      string `json:"context"`
}

type Jar struct {
	artifactBase `json:",inline"`
	SourcePath   string `json:"sourcepath"`
	TargetRepo   string `json:"targetrepo"`
	TargetPath   string `json:"targetpath"`
}

type Maven struct {
	artifactBase `json:",inline"`
	SourcePath   string `json:"sourcepath"`
	TargetRepo   string `json:"targetrepo"`
}

func (a *Artifact) MarshalJSON() ([]byte, error) {
	switch {
	case a.Docker != nil:
		return json.Marshal(a.Docker)
	case a.File != nil:
		return json.Marshal(a.File)
	case a.Jar != nil:
		return json.Marshal(a.Jar)
	case a.Maven != nil:
		return json.Marshal(a.Maven)
	default:
		panic("unknown artifact type")
	}
}

func (a *Artifact) UnmarshalJSON(data []byte) error {
	m := map[string]interface{}{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	t, ok := m["type"]
	if !ok {
		return errors.New("artifact missing 'type' field")
	}
	typestr, ok := t.(string)
	if !ok {
		return errors.New("unknown format for 'type' field")
	}

	switch typestr {
	case TypeDocker:
		d := &Docker{}
		if err := json.Unmarshal(data, d); err != nil {
			return err
		}
		d.Type = typestr
		a.Docker = d
	case TypeFile:
		f := &File{}
		if err := json.Unmarshal(data, f); err != nil {
			return err
		}
		f.Type = typestr
		a.File = f
	case TypeJar:
		j := &Jar{}
		if err := json.Unmarshal(data, j); err != nil {
			return err
		}
		j.Type = typestr
		a.Jar = j
	case TypeMaven:
		m := &Maven{}
		if err := json.Unmarshal(data, m); err != nil {
			return err
		}
		m.Type = typestr
		a.Maven = m
	default:
		return errors.New("unknown value for 'type' field")
	}

	return nil
}

func (a Artifact) Validate() error {
	found := []string{}

	if a.File != nil {
		found = append(found, "file")
	}

	if a.Docker != nil {
		found = append(found, "docker")
	}

	if a.Jar != nil {
		found = append(found, "jar")
	}

	if a.Maven != nil {
		found = append(found, "maven")
	}

	if len(found) == 0 {
		return fmt.Errorf("missing artifact configuration, please specify one of: docker/file/jar/maven)")
	}

	if len(found) > 1 {
		return fmt.Errorf("invalid artifact configuration, please specify one of: %v", strings.Join(found, "/"))
	}

	err := valid.ValidateStruct(&a,
		valid.Field(&a.File),
		valid.Field(&a.Docker),
		valid.Field(&a.Jar),
		valid.Field(&a.Maven),
	)
	return err
}

func (a Artifact) Type() string {
	return a.base().Type
}

func (a Artifact) Name() string {
	return a.base().Name
}

func (a Artifact) Labels() map[string]string {
	return a.base().Labels
}

func (a Artifact) base() artifactBase {
	switch {
	case a.Docker != nil:
		return a.Docker.artifactBase
	case a.File != nil:
		return a.File.artifactBase
	case a.Jar != nil:
		return a.Jar.artifactBase
	case a.Maven != nil:
		return a.Maven.artifactBase
	default:
		panic("unknown artifact type")
	}
}

func (j Jar) Validate() error {
	return valid.ValidateStruct(&j,
		valid.Field(&j.Type, valid.Required),
		valid.Field(&j.SourcePath, valid.Required),
		valid.Field(&j.TargetPath, valid.Required),
		valid.Field(&j.TargetRepo, valid.Required),
	)
}

func (f File) Validate() error {
	return valid.ValidateStruct(&f,
		valid.Field(&f.Type, valid.Required),
		valid.Field(&f.SourcePath, valid.Required),
		valid.Field(&f.TargetPath, valid.Required),
		valid.Field(&f.TargetRepo, valid.Required),
	)
}

func (m Maven) Validate() error {
	return valid.ValidateStruct(&m,
		valid.Field(&m.Type, valid.Required),
		valid.Field(&m.SourcePath, valid.Required),
		valid.Field(&m.TargetRepo, valid.Required),
	)
}

func (d Docker) Validate() error {
	return valid.ValidateStruct(&d,
		valid.Field(&d.Type, valid.Required),
		valid.Field(&d.Dockerfile, valid.Required),
	)
}
