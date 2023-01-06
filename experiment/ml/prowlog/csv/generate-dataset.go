/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main will create a CSV dataset by reading the specified zip file.
//
// Zip file is assumed to be created by generate-dataset.go, following this format:
//   TRAIN/labelA/foo.txt
//   VALIDATION/labelB/bar.txt
//   TEST/labelA/whatever.txt
// aka <partition>/<label>/<name>, or alternatively leaving out the parition:
//   labelA/foo.txt
//   labelB/bar.txt
//   labelA/whatever.txt
//
// The corresponding CSV file rows will look like:
//   TRAIN,"hello world",labelA
//   VALIDATION,"contents of bar",labelB
//   TEST,"more interesting stuff",labelA
// aka <parition>,<content of file>,<label>, possibly leaving the partition column blank:
//   ,"hello world",labelA
//   ,"contents of bar",labelB
//   ,"more interesting stuff",labelA
package main

import (
	"archive/zip"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"unicode/utf8"

	"bitbucket.org/creachadair/stringset"
)

var (
	input           = flag.String("input", "", "Consume a dataset.zip created by generate-dataset.go")
	output          = flag.String("output", "", "Output to the following .csv file")
	quiet           = flag.Bool("quiet", false, "Quiet mode; does not log per-row info")
	allowZero       = flag.Bool("zeros", false, "Allow NULL bytes when set")
	allowDuplicates = flag.Bool("duplicates", false, "Allow duplicated text context")
	allowESC        = flag.Bool("escapes", false, "Allow escape characters")
)

func main() {
	flag.Parse()

	if *input == "" {
		flag.Usage()
		log.Fatal("--input missing")
	}

	if *output == "" {
		flag.Usage()
		log.Fatal("--output missing")
	}

	reader, err := zip.OpenReader(*input)
	if err != nil {
		log.Fatal("Failed to open input file", *input, err)
	}

	of, err := os.Create(*output)
	if err != nil {
		log.Fatal("Failed to open output file", *input, err)
	}

	w := csv.NewWriter(of)

	var existing stringset.Set

	for i, f := range reader.File {
		if !strings.HasSuffix(f.Name, ".txt") {
			log.Println("Ignoring non .txt file", f.Name)
			continue
		}
		dataset, label, err := parseName(f.Name)
		if err != nil {
			log.Println("Failed to parse name", f.Name, err)
			continue
		}
		zf, err := reader.Open(f.Name)
		if err != nil {
			log.Println("Failed to open example", f.Name, err)
			continue
		}
		defer zf.Close()
		b, err := io.ReadAll(zf)
		if err != nil {
			log.Println("Failed to read eample", f.Name, err)
			continue
		}
		if !utf8.Valid(b) {
			log.Fatal("Invalid utf-8", f.Name)
		}
		s := string(b)
		if !*allowZero {
			s = strings.ReplaceAll(s, "\x00", " ")
		}
		if !*allowESC {
			s = strings.ReplaceAll(s, "\x1b", "ESC") // ESC
		}
		if !existing.Add(s) {
			log.Println("Duplicated example", i, f.Name)
			if !*allowDuplicates {
				continue
			}
		}
		record := []string{dataset, s, label}
		if !*quiet {
			log.Println(i, f.Name, dataset, label)
		}
		if err := w.Write(record); err != nil {
			log.Println("Failed to write record", f.Name, err)
			continue
		}
	}
	if err := reader.Close(); err != nil {
		log.Fatal("Failed to close", *input, err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal("Failed to flush csv", *output, err)
	}

	if err := of.Close(); err != nil {
		log.Fatal("Failed to close csv", *output, err)
	}

	if err := validate(*output); err != nil {
		log.Fatal("Corrupted output", *output, err)
	}

	log.Println("Successfully converted dataset", *input, *output)
}

func parseName(name string) (string, string, error) {
	parts := strings.SplitN(name, "/", 3)

	switch len(parts) {
	case 3: // TRAIN/label/foo.txt
		switch parts[0] {
		case "TRAIN", "VALIDATION", "TEST":
			return parts[0], parts[1], nil
		}
		return "", parts[0], nil
	case 2: // label/foo.txt
		return "", parts[0], nil
	default:
		return "", "", errors.New("format is DATASET/label/name.txt")
	}
}

func validate(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("open: %v", err)
	}
	r := csv.NewReader(f)
	var idx int
	for {
		idx++
		_, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%d: %v", idx, err)
		}
	}
	return f.Close()
}
