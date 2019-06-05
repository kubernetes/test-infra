package awsapi

import (
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/sirupsen/logrus"
)

type Client struct {
	session *session.Session
}

func NewClient(o ClientOption) (*Client, error) {
	sharedConfigState := session.SharedConfigDisable
	var sharedConfigFiles []string

	// ignore the gcs CredentialsFile, but assume that the config file is next to it and use it to get the region
	if o.CredentialsFile != "" {
		dir, _ := filepath.Split(o.CredentialsFile)
		configFile := filepath.Join(dir, "config")

		if _, err := os.Stat(configFile); err != nil {
			logrus.Warn("Config file not found: " + configFile)
		} else {
			sharedConfigState = session.SharedConfigEnable
			sharedConfigFiles = []string{configFile}
		}
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: sharedConfigState,
		SharedConfigFiles: sharedConfigFiles,
	})

	if err != nil {
		// Handle Session creation error
		return nil, err
	}

	return &Client{sess}, err
}

func (c *Client) Bucket(name string) *BucketHandle {
	return &BucketHandle{
		client: c,
		bucket: name,
	}
}
