package tagutil

import (
	"fmt"
	"sort"

	versioner "k8s.io/test-infra/prow/custom-reporter/guest-test-infra/version"
)

// tags is a list of tag name; returns the latest version tag
func GetLatestVersionTag(tags []string) (versioner.NonSemanticVer, error) {
	var validTags []versioner.NonSemanticVer
	for _, tag := range tags {
		v, err := versioner.NewNonSemanticVer(tag)
		if err == nil {
			validTags = append(validTags, *v)
		}
	}
	if len(validTags) == 0 {
		return versioner.NonSemanticVer{}, fmt.Errorf("No valid version tags found")
	}
	sort.Sort(versioner.VersionSorter(validTags))
	// return the last element since the sorter sorts in increasing order
	return validTags[len(validTags)-1], nil
}