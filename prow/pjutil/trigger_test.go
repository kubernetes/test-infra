package pjutil

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pjapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowconfig "k8s.io/test-infra/prow/config"
)

type fakeJobResult struct {
	err error
}

func (v *fakeJobResult) toJSON() ([]byte, error) {
	if v.err != nil {
		return nil, v.err
	}
	return []byte(`null`), v.err
}

func Test_writeResultOutput(t *testing.T) {
	fileSystem := afero.NewMemMapFs()
	afs := afero.Afero{Fs: fileSystem}
	for _, path := range []string{"/path/to/"} {
		_ = afs.MkdirAll(path, 0)
	}
	prowJob := &pjapi.ProwJob{
		Status: pjapi.ProwJobStatus{
			State:   pjapi.SuccessState,
			URL:     "http://example.com/result",
			BuildID: "1",
		},
	}

	prowJobResult := prowjobResult{
		Status:       prowJob.Status.State,
		ArtifactsURL: "",
		URL:          prowJob.Status.URL,
	}

	type args struct {
		prowJobResult prowjobResult
		outputPath    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "writes file when output path exists and is writable ",
			args: args{
				prowJobResult: prowJobResult,
				outputPath:    "/path/to/outputFile.json",
			},
			wantErr: false,
		},
		{
			name: "writes file when output path doesn't exist, but can be created",
			args: args{
				prowJobResult: prowJobResult,
				outputPath:    "/path/to/outputFile.json",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := writeResultOutput(&tt.args.prowJobResult, tt.args.outputPath, fileSystem); (err != nil) != tt.wantErr {
				t.Errorf("writeResultOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_writeResultOutput_FileSystemFailures(t *testing.T) {
	fileSystem := afero.NewMemMapFs()
	afs := afero.Afero{Fs: fileSystem}
	for _, path := range []string{"/path/to/"} {
		_ = afs.MkdirAll(path, 0)
	}

	// Set our file system to read-only to mimic trying to write to
	// areas without permission
	fileSystem = afero.NewReadOnlyFs(afero.NewMemMapFs())

	type args struct {
		prowjobResult prowjobResult
		outputPath    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "returns error when output path exists, but isn't writable",
			args: args{
				prowjobResult: prowjobResult{},
				outputPath:    "/path/to/output.json",
			},
			wantErr: true,
		},
		{
			name: "returns error when output path doesn't exist, and cannot be created",
			args: args{
				prowjobResult: prowjobResult{},
				outputPath:    "/some/other/output.json",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := writeResultOutput(&tt.args.prowjobResult, tt.args.outputPath, fileSystem); (err != nil) != tt.wantErr {
				t.Errorf("writeResultOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_writeResultOutput_JsonMarshalFailure(t *testing.T) {
	fileSystem := afero.NewMemMapFs()
	type args struct {
		prowjobResult JobResult
		outputPath    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "returns error when unable to marshal prowjobResult struct",
			args: args{
				prowjobResult: &fakeJobResult{err: errors.Errorf("Unable to marshal")},
				outputPath:    "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := writeResultOutput(tt.args.prowjobResult, tt.args.outputPath, fileSystem); (err != nil) != tt.wantErr {
				t.Errorf("writeResultOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_toJson(t *testing.T) {
	type fields struct {
		Status       pjapi.ProwJobState
		ArtifactsURL string
		URL          string
	}
	tests := []struct {
		name    string
		fields  fields
		want    []byte
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				Status:       "success",
				ArtifactsURL: "http://example.com/jobName/1/artifacts",
				URL:          "http://example.com/jobName/1/",
			},
			want: []byte(`{
    "status": "success",
    "prowjob_artifacts_url": "http://example.com/jobName/1/artifacts",
    "prowjob_url": "http://example.com/jobName/1/"
}`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &prowjobResult{
				Status:       tt.fields.Status,
				ArtifactsURL: tt.fields.ArtifactsURL,
				URL:          tt.fields.URL,
			}
			got, err := p.toJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("toJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toJSON() got = \n%v, want \n%v", string(got), string(tt.want))
			}
		})
	}
}

func Test_getJobArtifactsURL(t *testing.T) {
	org := "redhat-operator-ecosystem"
	repo := "playground"
	bucket := "origin-ci-test"
	browserPrefix := "https://gcsweb-ci.svc.ci.openshift.org/gcs/"
	jobName := "periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws"

	prowConfig := &prowconfig.Config{
		JobConfig: prowconfig.JobConfig{},
		ProwConfig: prowconfig.ProwConfig{
			Plank: config.Plank{
				Controller: prowconfig.Controller{},
				DefaultDecorationConfigs: map[string]*pjapi.DecorationConfig{
					fmt.Sprintf("%s/%s", org, repo): {GCSConfiguration: &pjapi.GCSConfiguration{Bucket: bucket}},
				},
			},
			Deck: prowconfig.Deck{
				Spyglass: prowconfig.Spyglass{GCSBrowserPrefix: browserPrefix},
			},
		},
	}
	type args struct {
		prowJob *pjapi.ProwJob
		config  *prowconfig.Config
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Returns artifacts URL when we have .Spec.Ref",
			args: args{
				prowJob: &pjapi.ProwJob{
					TypeMeta:   v1.TypeMeta{},
					ObjectMeta: v1.ObjectMeta{Name: "jobWithSpecRef"},
					Spec: pjapi.ProwJobSpec{
						ExtraRefs: nil,
						Job:       jobName,
						Refs:      &pjapi.Refs{Org: org, Repo: repo},
						Type:      "periodic",
					},
					Status: pjapi.ProwJobStatus{State: "success", BuildID: "100"},
				},
				config: prowConfig,
			},
			want: "https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/logs/periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws/100",
		},
		{
			name: "Returns artifacts URL when we have Spec.ExtraRefs",
			args: args{
				prowJob: &pjapi.ProwJob{
					TypeMeta:   v1.TypeMeta{},
					ObjectMeta: v1.ObjectMeta{Name: "jobWithExtraRef"},
					Spec: pjapi.ProwJobSpec{
						ExtraRefs: []pjapi.Refs{
							{Org: org, Repo: repo},
							{Org: "org2", Repo: "repo2"},
						},
						Job:  jobName,
						Refs: nil,
						Type: "periodic",
					},
					Status: pjapi.ProwJobStatus{State: "success", BuildID: "101"},
				},
				config: prowConfig,
			},
			want: "https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/logs/periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws/101",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getJobArtifactsURL(tt.args.prowJob, tt.args.config); got != tt.want {
				t.Errorf("getJobArtifactsURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
