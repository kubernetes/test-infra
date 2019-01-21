package awsapi

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iterator"
	"testing"
)

func TestS3ObjectIterator(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")

//	it := b.Objects(&Query {Prefix: "pr-logs/", Delimiter:"/"})
	it := b.Objects(&Query {Prefix: "", Delimiter:"/"})
//	it := b.Objects(&Query {Prefix: "pr-logs/"})
	//it := b.Objects(&Query {Delimiter: "/"})

	for {
		attrs, err := it.Next()

		if attrs != nil {
			fmt.Println(attrs.Name)
		}

		if err == iterator.Done {
			break
		}

	}
}
