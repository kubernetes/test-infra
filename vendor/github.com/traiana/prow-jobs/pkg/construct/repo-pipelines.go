package construct

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"

	civ1beta2 "github.com/traiana/okro/okro/api/ci/v1beta2"
)

func BuildRepoPipelines(path string) ([]*civ1beta2.RepoPipelines, error) {
	var allRepoPipelines []*civ1beta2.RepoPipelines
	err := filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if strings.ToLower(filepath.Ext(path)) != ".yaml" {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open '%s': %v", path, err)
			}
			defer file.Close()
			decoder := yaml.NewYAMLToJSONDecoder(file)
			for {
				var repoPipelines *civ1beta2.RepoPipelines
				if decodeErr := decoder.Decode(&repoPipelines); decodeErr != nil {
					if decodeErr == io.EOF {
						break
					}
					return fmt.Errorf("failed to parse repo pipelines in '%s': %v", path, err)
				}
				if err := repoPipelines.Validate(); err != nil {
					return fmt.Errorf("failed to validate repo pipelines in '%s': %v", path, err)
				}
				allRepoPipelines = append(allRepoPipelines, repoPipelines)
			}
			return nil
		})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk dir: %v", err)
	}
	return allRepoPipelines, nil
}
