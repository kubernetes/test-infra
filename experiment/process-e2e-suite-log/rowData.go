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

// RowData will hold common information for each row.
// filename: the name of the file processed.
// testDescription: description of the testcase.
// columns: map holding the column as a key and the value as the corresponding value for that column.
type rowData struct {
	fileName        string
	testDescription string
	columns         map[string]string
}

func newRowData(fileName string, testDescription string) *rowData {
	return &rowData{
		fileName:        fileName,
		testDescription: testDescription,
		columns:         make(map[string]string),
	}
}

func (rowData *rowData) addColumn(key string, value string) {
	rowData.columns[key] = value
}
