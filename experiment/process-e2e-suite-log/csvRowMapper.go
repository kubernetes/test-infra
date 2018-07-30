/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"bytes"
)

// CSVRowMapper is a struct that holds
// 1) separatorCharacterPtr: is the character used to delimit each item in the row.
// 2) keys: array (ordered) with keys to build the row consistently.
type csvRowMapper struct {
	separatorCharacterPtr *string
	keys                  []string
}

func newCSVRowMapper(separatorCharacterPtr *string, keys []string) *csvRowMapper {
	return &csvRowMapper{
		separatorCharacterPtr: separatorCharacterPtr,
		keys: keys,
	}
}

func (rowMapper *csvRowMapper) toRow(rowDataPtr *rowData) string {
	var headerBuffer bytes.Buffer

	headerBuffer.WriteString(rowDataPtr.fileName)
	headerBuffer.WriteString(*(rowMapper.separatorCharacterPtr))
	headerBuffer.WriteString(rowDataPtr.testDescription)

	for _, key := range rowMapper.keys {
		headerBuffer.WriteString(*(rowMapper.separatorCharacterPtr))
		headerBuffer.WriteString(rowDataPtr.columns[key])
	}
	return headerBuffer.String()
}
