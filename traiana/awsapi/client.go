package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

type Client struct {
	session *session.Session
}

func NewClient(o ClientOption) (*Client, error) {
	config := aws.Config{}
	var sharedConfigFiles []string
	sharedConfigState := session.SharedConfigDisable

	// use the gcs CredentialsFile for AWS and assume that the config file is next to it
	if o.CredentialsFile != "" {
		config.Credentials = credentials.NewSharedCredentials(o.CredentialsFile, "default")

		//k create secret generic aws-secret --from-file=service-account.json --from-file=config
		dir, _ := filepath.Split(o.CredentialsFile)
		configFile := filepath.Join(dir, "config")

		if _, err := os.Stat(configFile); err != nil {
			logrus.Warn("Config file not found: " + configFile)
		} else {
			sharedConfigFiles = append(sharedConfigFiles, configFile)
			sharedConfigState = session.SharedConfigEnable
		}
	}

	opts := session.Options {
		Config: config,
		SharedConfigFiles: sharedConfigFiles,
		SharedConfigState: sharedConfigState,
	}

	session, err := session.NewSessionWithOptions(opts)

	if err != nil {
		// Handle Session creation error
		return nil, err
	}

	//sess.Handlers.Send.PushFront(func(r *request.Request) {
	// Log every request made and its payload
	//logger.Println("Request: %s/%s, Payload: %s",
	//	r.ClientInfo.ServiceName, r.Operation, r.Params)

	return &Client{session}, err
}

func (c *Client) Bucket(name string) *BucketHandle {
	return &BucketHandle{
		client: c,
		bucket: name,
	}
}