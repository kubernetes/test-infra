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

package mungers

import (
	"fmt"
	"math/rand"
	"testing"

	"k8s.io/kubernetes/pkg/util/sets"
)

const (
	RANDOM_SEED = int64(30)
	NUM_TRIALS  = 10
)

func buildTestMap() map[string]sets.String {
	testSet := make(map[string]sets.String)
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for _, r := range chars {
		testSet[fmt.Sprintf("%c", r)] = sets.NewString()
	}
	return testSet
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// For a single PR, make sure the random order is the same across multiple runs
func TestDeterministic(t *testing.T) {
	testMap := buildTestMap()
	rand.Seed(RANDOM_SEED)
	// this is the expected order hardcoded as calculated for RANDOM_SEED=30
	expectedOrder := []string{"Q", "U", "M", "P", "O", "S", "N", "R", "Y", "Z", "A", "W", "E", "D", "G", "T", "B", "C", "V", "X", "K", "I", "H", "F", "J", "L"}
	for i := 0; i < NUM_TRIALS; i++ {
		// this tests getting the same random permutation for the same PR
		rand.Seed(RANDOM_SEED)
		calculatedOrder := getRandomizedKeys(testMap)
		if !sliceEq(calculatedOrder, expectedOrder) {
			t.Error("Didn't get the same random order of keys across multiples runs")
			t.Errorf("Run 1 %v\n", expectedOrder)
			t.Errorf("Run %v %v\n", i, calculatedOrder)
		}
	}

}

// For different PRs with the same approvers, make sure we get a random ordering each time
// Technically, it's possible that we generate the same random perm correctly twice, but it's highly impropable
// so if it happens more than twice, we can assume an error
func TestRandom(t *testing.T) {
	testMap := buildTestMap()
	rand.Seed(RANDOM_SEED)
	orders := [][]string{}
	for i := 0; i < NUM_TRIALS; i++ {
		rand.Seed(int64(i))
		orders = append(orders, getRandomizedKeys(testMap))
	}
	equalCount := 0
	for i := 0; i < NUM_TRIALS-1; i++ {
		for j := i + 1; j < NUM_TRIALS; j++ {
			if sliceEq(orders[i], orders[j]) {
				equalCount++
				if equalCount > 2 {
					t.Error("Randomization isn't working well")
					t.Errorf("Found this pattern multiple times %v", orders[i])
				}
			}
		}
	}
}
