package awsupload

import (
	"github.com/gophercloud/gophercloud/testhelper"
	"testing"
)

func Test_Run(t *testing.T) {
	/*gcsupload.Options{
		Items:              nil,
		SubDir:             "",
		GCSConfiguration:   nil,
		GcsCredentialsFile: "",
		DryRun:             false,
	}
*/
	err := Run(nil, false)

	testhelper.AssertNoErr(t, err)
}
