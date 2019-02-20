package construct

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

const (
	domainFile = "domain.yaml"
	realmFile  = "realm.yaml"
)

func Domain(path string) (*okrov1beta2.Domain, error) {
	fileInfo, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir '%s': %v", path, err)
	}
	var domain *okrov1beta2.Domain
	var realms []*okrov1beta2.Realm
	for _, file := range fileInfo {
		fileName := file.Name()
		filePath := filepath.Join(path, fileName)
		if strings.HasPrefix(fileName, ".") {
			continue
		}
		if file.IsDir() {
			realm, err := constructRealm(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to construct realm '%s': %v", fileName, err)
			}
			realms = append(realms, realm)
		} else if strings.ToLower(fileName) == domainFile {
			if err := decodeInto(filePath, &domain); err != nil {
				return nil, fmt.Errorf("failed to parse '%s': %v", fileName, err)
			}
		}
	}
	domain.Realms = realms
	return domain, nil
}

func decodeInto(path string, target interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := yaml.NewYAMLToJSONDecoder(file)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func constructRealm(path string) (*okrov1beta2.Realm, error) {
	var realm *okrov1beta2.Realm
	realmFilePath := filepath.Join(path, realmFile)
	if err := decodeInto(realmFilePath, &realm); err != nil {
		return nil, fmt.Errorf("failed to parse '%s': %v", realmFilePath, err)
	}
	dirName := filepath.Base(path)
	if realm.Name != dirName {
		return nil, fmt.Errorf("realm name '%s' does not match directory name '%s'", realm.Name, dirName)
	}

	var allTasks []*okrov1beta2.Task
	err := filepath.Walk(filepath.Join(path, "tasks"),
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
				var task *okrov1beta2.Task
				if decodeErr := decoder.Decode(&task); decodeErr != nil {
					if decodeErr == io.EOF {
						break
					}
					return fmt.Errorf("failed to parse task in '%s': %v", path, err)
				}
				// TODO: validate task (currently requires prior normalization)
				allTasks = append(allTasks, task)
			}
			return nil
		})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk tasks dir: %v", err)
	}

	var allTopicProjs []*okrov1beta2.TopicProjection
	err = filepath.Walk(filepath.Join(path, "resources", "topics"),
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
				var topicProj *okrov1beta2.TopicProjection
				if decodeErr := decoder.Decode(&topicProj); decodeErr != nil {
					if decodeErr == io.EOF {
						break
					}
					return fmt.Errorf("failed to parse topic projection in '%s': %v", path, err)
				}
				// TODO: validate topic projection
				allTopicProjs = append(allTopicProjs, topicProj)
			}
			return nil
		})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk resources/topics dir: %v", err)
	}

	realm.Tasks = allTasks
	realm.Resources = okrov1beta2.Resources{
		Topics: allTopicProjs,
	}
	// TODO: validate realm (currently requires prior normalization)
	return realm, nil
}
