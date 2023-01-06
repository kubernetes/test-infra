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

package dummybenchmarks

import "testing"

// TestDontRun is for validating that we are not running tests with benchmarks.
func TestDontRun(t *testing.T) {
	t.Log("This is a Test not a Benchmark!")
}

// The first group of benchmarks in this file include "Core" in their names so
// that they may be selected with a regexp when running integration tests.
// This allows us to avoid running all the benchmarks to reduce testing time.

func BenchmarkCoreSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DoTheThing()
	}
}

func BenchmarkCoreAllocsAndBytes(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DoTheThing()
		b.SetBytes(20)
	}
}

func BenchmarkCoreParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			DoTheThing()
		}
	})
}

func BenchmarkCoreLog(b *testing.B) {
	b.Logf("About to DoTheThing() x%d.", b.N)
	for i := 0; i < b.N; i++ {
		DoTheThing()
	}
}

func BenchmarkCoreSkip(b *testing.B) {
	b.Skip("This Benchmark is skipped.")
}

func BenchmarkCoreSkipNow(b *testing.B) {
	b.SkipNow()
}

func BenchmarkCoreError(b *testing.B) {
	b.Error("Early Benchmark error.")
	BenchmarkCoreLog(b)
}

func BenchmarkCoreFatal(b *testing.B) {
	b.Fatal("This Benchmark failed.")
}

func BenchmarkCoreFailNow(b *testing.B) {
	b.FailNow()
}

func BenchmarkCoreNestedShallow(b *testing.B) {
	b.Run("simple", BenchmarkCoreSimple)
	b.Run("parallel", BenchmarkCoreParallel)
}

// The following benchmarks produce output to check additional edge cases, that
// are already somewhat covered by the preceding benchmarks.
// They do not include "Core" so that they may be omitted when running
// integration tests to reduce runtimes.

func Benchmark(b *testing.B) {
	BenchmarkCoreSimple(b)
}

func BenchmarkReportAllocs(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		DoTheThing()
	}
}

func BenchmarkSetBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DoTheThing()
		b.SetBytes(20)
	}
}

func BenchmarkNestedDeep(b *testing.B) {
	b.Run("1", func(b1 *testing.B) {
		b.Run("1 simple", BenchmarkCoreSimple)
		b.Run("1 parallel", BenchmarkCoreParallel)

		b.Run("2", func(b2 *testing.B) {
			b.Run("3A", func(b3 *testing.B) {
				b.Run("3A simple", BenchmarkCoreSimple)
			})
			b.Run("3B", func(b3 *testing.B) {
				b.Run("3B parallel", BenchmarkCoreParallel)
			})
		})
	})
}
