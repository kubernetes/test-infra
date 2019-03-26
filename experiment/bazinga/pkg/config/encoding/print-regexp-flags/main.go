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

package main

import (
	"fmt"
	"regexp/syntax"
)

func main() {
	fmt.Printf(`FoldCase         % 4d
Literal          % 4d
ClassNL          % 4d
DotNL            % 4d
OneLine          % 4d
NonGreedy        % 4d
PerlX            % 4d
UnicodeGroups    % 4d
WasDollar        % 4d
Simple           % 4d
MatchNL          % 4d
Perl             % 4d
POSIX            % 4d
`,
		syntax.FoldCase,
		syntax.Literal,
		syntax.ClassNL,
		syntax.DotNL,
		syntax.OneLine,
		syntax.NonGreedy,
		syntax.PerlX,
		syntax.UnicodeGroups,
		syntax.WasDollar,
		syntax.Simple,
		syntax.MatchNL,
		syntax.Perl,
		syntax.POSIX)
}
