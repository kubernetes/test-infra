package construct

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

func Catalog(path string) (*okrov1beta2.Catalog, error) {
	var allAPIGroups []*okrov1beta2.APIGroup
	err := filepath.Walk(filepath.Join(path, "apis"),
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
				var apiGroup *okrov1beta2.APIGroup
				if decodeErr := decoder.Decode(&apiGroup); decodeErr != nil {
					if decodeErr == io.EOF {
						break
					}
					return fmt.Errorf("failed to parse API group in '%s': %v", path, err)
				}
				if err := apiGroup.Validate(); err != nil {
					return fmt.Errorf("failed to validate API group in '%s': %v", path, err)
				}
				allAPIGroups = append(allAPIGroups, apiGroup)
			}
			return nil
		})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk apis dir: %v", err)
	}

	var allTopics []*okrov1beta2.Topic
	err = filepath.Walk(filepath.Join(path, "topics"),
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
				var topic *okrov1beta2.Topic
				if decodeErr := decoder.Decode(&topic); decodeErr != nil {
					if decodeErr == io.EOF {
						break
					}
					return fmt.Errorf("failed to parse topic in '%s': %v", path, err)
				}
				if err := topic.Validate(); err != nil {
					return fmt.Errorf("failed to validate topic in '%s': %v", path, err)
				}
				allTopics = append(allTopics, topic)
			}
			return nil
		})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk topics dir: %v", err)
	}

	return &okrov1beta2.Catalog{
		APIGroups: allAPIGroups,
		Topics:    allTopics,
	}, nil
}
