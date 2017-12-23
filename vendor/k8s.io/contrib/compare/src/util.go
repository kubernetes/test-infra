/*
Copyright 2015 The Kubernetes Authors.

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

package src

import (
	"fmt"
	"io"

	"github.com/daviddengcn/go-colortext"
	"math"
)

// ResetColor is a slightly modified version of daviddengcn/go-colortext, so it'll work well with io.Writer
func ResetColor(writer io.Writer) {
	fmt.Fprint(writer, "\x1b[0m")
}

// ChangeColor is a slightly modified version of daviddengcn/go-colortext, so it'll work well with io.Writer
func ChangeColor(color ct.Color, writer io.Writer) {
	if color == ct.None {
		return
	}
	s := ""
	if color != ct.None {
		s = fmt.Sprintf("%s%d", s, 30+(int)(color-ct.Black))
	}

	s = "\x1b[0;" + s + "m"
	fmt.Fprint(writer, s)
}

func changeColorFloat64AndWrite(data, baseline, allowedVariance float64, enableOutputColoring bool, writer io.Writer) {
	if enableOutputColoring {
		if data > baseline*allowedVariance {
			ChangeColor(ct.Red, writer)
		} else if math.IsNaN(data) {
			ChangeColor(ct.Yellow, writer)
		} else {
			// to keep tabwriter happy...
			ChangeColor(ct.White, writer)
		}
	}
	fmt.Fprintf(writer, "%.2f", data)
	if enableOutputColoring {
		ResetColor(writer)
	}
}

func leq(left, right float64) bool {
	return left <= right || (math.IsNaN(left) && math.IsNaN(right))
}

// Prints build number injecting dummy colors to make cell align again
func printBuildNumber(buildNumber int, writer io.Writer, enableOutputColoring bool) {
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
	}
	fmt.Fprintf(writer, "%v", buildNumber)
	if enableOutputColoring {
		ResetColor(writer)
	}
}
