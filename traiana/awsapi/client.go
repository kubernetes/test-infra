package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

type Client struct {
	session *session.Session
}

func NewClient(o ClientOption) (*Client, error) {
	session, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewSharedCredentials(o.CredentialsFile, "default"),
		Region: aws.String("eu-west-1"),
	})

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