package awsapi

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_S3Download(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("dev-okro-io")
	o := b.Object("lala")

	reader := o.NewRangeReader(context.Background(), 0, 5)

	buf := make([]byte, 5)

	n, err := reader.Read(buf)
	assert.NoError(t, err)

	assert.Equal(t, 5, n)

	assert.NotEqual(t, []byte {0,0,0,0,0}, buf)


}

/*func Test_S3Download(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("dev-okro-io")

	file, err := os.Create("/Users/Traiana/alexa/Downloads/d.txt")
	assert.NoError(t, err)

	defer file.Close()

	n, err := S3Download(file, b, "lala", 0, 2)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), n)
}*/