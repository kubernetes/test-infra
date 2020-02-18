/*
Copyright 2019 The Kubernetes Authors.

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

package multiline_comments

type Multiline struct {
	// StringField1 comment
	// second line
	// third line
	StringField1 string `json:"string1"`

	/* StringField2 comment
	second line
	third line
	*/
	StringField2 string `json:"string2"`

	/* StringField3 comment
	second line
	third line
	*/
	StringField3 string `json:"string3"`

	//
	//
	// Paragraph line
	//
	//
	StringField4 string `json:"string4"`

	/*

		Paragraph block

	*/
	StringField5 string `json:"string5"`

	/*
		Tab	Tab		TabTab
	*/
	StringField6 string `json:"string6"`
}
