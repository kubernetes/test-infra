package eksconfig

import (
	"fmt"
	"os"
)

func newInt(v int) *int {
	return &v
}

func exist(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// genS3URL returns S3 URL path.
// e.g. https://s3-us-west-2.amazonaws.com/awstester-20180925/hello-world
func genS3URL(region, bucket, s3Path string) string {
	return fmt.Sprintf("https://s3-%s.amazonaws.com/%s/%s", region, bucket, s3Path)
}
