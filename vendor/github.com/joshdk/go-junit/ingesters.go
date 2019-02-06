package junit

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// IngestDir will search the given directory for XML files and return a slice
// of all contained JUnit test suite definitions.
func IngestDir(directory string) ([]Suite, error) {
	var filenames []string

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Add all regular files that end with ".xml"
		if info.Mode().IsRegular() && strings.HasSuffix(info.Name(), ".xml") {
			filenames = append(filenames, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return IngestFiles(filenames)
}

// IngestFiles will parse the given XML files and return a slice of all
// contained JUnit test suite definitions.
func IngestFiles(filenames []string) ([]Suite, error) {
	var all []Suite

	for _, filename := range filenames {
		suites, err := IngestFile(filename)
		if err != nil {
			return nil, err
		}
		all = append(all, suites...)
	}

	return all, nil
}

// IngestFile will parse the given XML file and return a slice of all contained
// JUnit test suite definitions.
func IngestFile(filename string) ([]Suite, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return Ingest(data)
}

// Ingest will parse the given XML data and return a slice of all contained
// JUnit test suite definitions.
func Ingest(data []byte) ([]Suite, error) {
	var (
		suiteChan = make(chan Suite)
		suites    []Suite
	)

	nodes, err := parse(data)
	if err != nil {
		return nil, err
	}

	go func() {
		findSuites(nodes, suiteChan)
		close(suiteChan)
	}()

	for suite := range suiteChan {
		suites = append(suites, suite)
	}

	return suites, nil
}
