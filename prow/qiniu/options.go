package qiniu

import (
	"encoding/json"
	"errors"
	"flag"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// NewOptions returns an empty Options with no nil fields.
func NewOptions() *Options {
	return &Options{
		GCSConfiguration: &prowapi.GCSConfiguration{},
	}
}

// Options exposes the configuration necessary
// for defining where in GCS an upload will land.
type Options struct {
	*prowapi.GCSConfiguration

	// Items are files or directories to upload.
	Items  []string `json:"items,omitempty"`
	DryRun bool     `json:"dry_run"`

	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`

	// domain used to download files from qiniu cloud
	Domain string `json:"domain"`
}

// Validate ensures that the set of options are
// self-consistent and valid.
func (o *Options) Validate() error {
	if !o.DryRun {
		if o.Bucket == "" {
			return errors.New("no Qiniu bucket was provided")
		}

		if o.AccessKey == "" || o.SecretKey == "" {
			return errors.New("no Qiniu secret was provided")
		}
	}

	return nil
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.Items = args
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable.
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}

// AddFlags adds flags to the FlagSet that populate
// the GCS upload options struct given.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.Bucket, "bucket", "", "bucket where the artifacts uploaded to")
	fs.StringVar(&o.AccessKey, "access-key", "", "key to access qiniu bucket")
	fs.StringVar(&o.SecretKey, "secret-key", "", "secret to access qiniu bucket")

	fs.BoolVar(&o.DryRun, "dry-run", true, "do not interact with cloud")
}
