package awsapi

import (
	"context"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"strconv"
	"testing"
)

func Test_RangeReader(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")
	o := b.Object("pr-logs/directory/test-presubmit/latest-build.txt")

	r := o.NewReader(context.Background())

	v, err := ioutil.ReadAll(r)
	assert.NoError(t, err)

	i, err := strconv.ParseInt(string(v), 10, 64)
	assert.NoError(t, err)
	assert.NotEqual(t, 0, i)
}