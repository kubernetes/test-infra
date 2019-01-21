package awsapi

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestS3GetObject(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")

	o := b.Object("f20.txt")

	att, err := o.Attrs()

	_ = att
}
