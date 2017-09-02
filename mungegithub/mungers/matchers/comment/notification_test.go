/*
Copyright 2016 The Kubernetes Authors.

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

package comment

import (
	"reflect"
	"testing"
)

func TestParseNotification(t *testing.T) {
	tests := []struct {
		notif   *Notification
		comment string
	}{
		{
			notif:   nil,
			comment: "I have nothing to do with a notification",
		},
		{
			notif:   nil,
			comment: " [NOTIF] Line can't start with space",
		},
		{
			notif:   nil,
			comment: "[NOTIF SOMETHING] Notif name can't have space",
		},
		{
			notif:   &Notification{Name: "NOTIF"},
			comment: "[NOTIF]",
		},
		{
			notif:   nil,
			comment: "Notif must be at the very beginning:\n[NOTIF]\nAnd something else...",
		},
		{
			notif:   &Notification{Name: "NOTIF", Arguments: "Valid notification"},
			comment: "[NOTIF] Valid notification",
		},
		{
			notif:   &Notification{Name: "NOTIF", Arguments: "Multiple Lines"},
			comment: "[NOTIF]   Multiple Lines  \nAnd something else...",
		},
		{
			notif:   &Notification{Name: "NOTIF", Arguments: "Notif name is upper-cased"},
			comment: "[notif] Notif name is upper-cased",
		},
		{
			notif:   &Notification{Name: "NOTIF", Arguments: "Arguments is trimmed"},
			comment: "[notif]     Arguments is trimmed   ",
		},
		{
			notif:   &Notification{Name: "NOTIF", Arguments: "Context is read", Context: "What a lovely context"},
			comment: "[notif]     Context is read \n\n  What a lovely context    ",
		},
	}

	for _, test := range tests {
		actualNotif := ParseNotification(&Comment{Body: &test.comment})
		if !reflect.DeepEqual(actualNotif, test.notif) {
			t.Errorf("%#v doesn't match expected notif: %#v", actualNotif, test.notif)
		}
	}
}

func TestStringNotification(t *testing.T) {
	tests := []struct {
		notif *Notification
		str   string
	}{
		{
			notif: &Notification{Name: "NOTIF"},
			str:   "[NOTIF]",
		},
		{
			notif: &Notification{Name: "NOTIF", Arguments: "Argument"},
			str:   "[NOTIF] Argument",
		},
		{
			notif: &Notification{Name: "NOTIF", Arguments: "Argument", Context: "Context"},
			str:   "[NOTIF] Argument\n\nContext",
		},
		{
			notif: &Notification{Name: "notif", Arguments: "  Argument  ", Context: "Context"},
			str:   "[NOTIF] Argument\n\nContext",
		},
	}

	for _, test := range tests {
		actualString := test.notif.String()
		if actualString != test.str {
			t.Error(actualString, "doesn't match expected string:", test.str)
		}
	}
}
