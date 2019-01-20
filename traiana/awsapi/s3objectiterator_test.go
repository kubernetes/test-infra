package awsapi

import (
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iterator"
	"fmt"
	"testing"
)

func TestS3ObjectIterator(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")

	it := b.Objects(&Query {Prefix: ""})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}

		fmt.Println(attrs.Name)
	}
}
