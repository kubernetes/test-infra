package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

//import "github.com/aws/aws-sdk-go/service/s3"

func NewSession() (*session.Session, error) {

	//sess, err := session.NewSession(&aws.Config{Region: aws.String("us-west-1")})
	//	_, err := sess.Config.Credentials.Get()

	session, err := session.NewSession(&aws.Config{Region: aws.String("eu-west-1")})
	if err != nil {
		// Handle Session creation error
		return nil, err
	}

	//sess.Handlers.Send.PushFront(func(r *request.Request) {
	// Log every request made and its payload
	//logger.Println("Request: %s/%s, Payload: %s",
	//	r.ClientInfo.ServiceName, r.Operation, r.Params)

	return session, err
}