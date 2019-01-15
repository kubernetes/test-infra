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

package history

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestHistory(t *testing.T) {
	var nowTime = time.Now()
	oldNow := now
	now = func() time.Time { return nowTime }
	defer func() { now = oldNow }()

	const logSizeLimit = 3
	nextTime := func() time.Time {
		nowTime = nowTime.Add(time.Minute)
		return nowTime
	}

	testMeta := func(num int, author string) prowapi.Pull {
		return prowapi.Pull{
			Number: num,
			Title:  fmt.Sprintf("PR #%d", num),
			SHA:    fmt.Sprintf("SHA for %d", num),
			Author: author,
		}
	}

	hist := New(logSizeLimit)
	time1 := nextTime()
	hist.Record("pool A", "TRIGGER", "sha A", "", []prowapi.Pull{testMeta(1, "bob")})
	nextTime()
	hist.Record("pool B", "MERGE", "sha B1", "", []prowapi.Pull{testMeta(2, "joe")})
	time3 := nextTime()
	hist.Record("pool B", "MERGE", "sha B2", "", []prowapi.Pull{testMeta(3, "jeff")})
	time4 := nextTime()
	hist.Record("pool B", "MERGE_BATCH", "sha B3", "", []prowapi.Pull{testMeta(4, "joe"), testMeta(5, "jim")})
	time5 := nextTime()
	hist.Record("pool C", "TRIGGER_BATCH", "sha C1", "", []prowapi.Pull{testMeta(6, "joe"), testMeta(8, "me")})
	time6 := nextTime()
	hist.Record("pool B", "TRIGGER", "sha B4", "", []prowapi.Pull{testMeta(7, "abe")})

	expected := map[string][]*Record{
		"pool A": {
			&Record{
				Time:    time1,
				BaseSHA: "sha A",
				Action:  "TRIGGER",
				Target: []prowapi.Pull{
					testMeta(1, "bob"),
				},
			},
		},
		"pool B": {
			&Record{
				Time:    time6,
				BaseSHA: "sha B4",
				Action:  "TRIGGER",
				Target: []prowapi.Pull{
					testMeta(7, "abe"),
				},
			},
			&Record{
				Time:    time4,
				BaseSHA: "sha B3",
				Action:  "MERGE_BATCH",
				Target: []prowapi.Pull{
					testMeta(4, "joe"),
					testMeta(5, "jim"),
				},
			},
			&Record{
				Time:    time3,
				BaseSHA: "sha B2",
				Action:  "MERGE",
				Target: []prowapi.Pull{
					testMeta(3, "jeff"),
				},
			},
		},
		"pool C": {
			&Record{
				Time:    time5,
				BaseSHA: "sha C1",
				Action:  "TRIGGER_BATCH",
				Target: []prowapi.Pull{
					testMeta(6, "joe"),
					testMeta(8, "me"),
				},
			},
		},
	}

	if got := hist.AllRecords(); !reflect.DeepEqual(got, expected) {
		es, _ := json.Marshal(expected)
		gs, _ := json.Marshal(got)
		t.Errorf("Expected history \n%s, but got \n%s.", es, gs)
		t.Logf("strs equal: %v.", string(es) == string(gs))
	}
}
