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

package cov

import (
	"errors"
	"fmt"
	"golang.org/x/tools/cover"
	"io"
)

// DumpProfile dumps the profiles given to writer in go coverage format.
func DumpProfile(profiles []*cover.Profile, writer io.Writer) error {
	if len(profiles) == 0 {
		return errors.New("can't write an empty profile")
	}
	if _, err := io.WriteString(writer, "mode: "+profiles[0].Mode+"\n"); err != nil {
		return err
	}
	for _, profile := range profiles {
		for _, block := range profile.Blocks {
			if _, err := fmt.Fprintf(writer, "%s:%d.%d,%d.%d %d %d\n", profile.FileName, block.StartLine, block.StartCol, block.EndLine, block.EndCol, block.NumStmt, block.Count); err != nil {
				return err
			}
		}
	}
	return nil
}

func deepCopyProfile(profile cover.Profile) cover.Profile {
	p := profile
	p.Blocks = make([]cover.ProfileBlock, len(profile.Blocks))
	copy(p.Blocks, profile.Blocks)
	return p
}

// blocksEqual returns true if the blocks refer to the same code, otherwise false.
// It does not care about Count.
func blocksEqual(a cover.ProfileBlock, b cover.ProfileBlock) bool {
	return a.StartCol == b.StartCol && a.StartLine == b.StartLine &&
		a.EndCol == b.EndCol && a.EndLine == b.EndLine && a.NumStmt == b.NumStmt
}

func ensureProfilesMatch(a *cover.Profile, b *cover.Profile) error {
	if a.FileName != b.FileName {
		return fmt.Errorf("coverage filename mismatch (%s vs %s)", a.FileName, b.FileName)
	}
	if len(a.Blocks) != len(b.Blocks) {
		return fmt.Errorf("file block count for %s mismatches (%d vs %d)", a.FileName, len(a.Blocks), len(b.Blocks))
	}
	if a.Mode != b.Mode {
		return fmt.Errorf("mode for %s mismatches (%s vs %s)", a.FileName, a.Mode, b.Mode)
	}
	for i, ba := range a.Blocks {
		bb := b.Blocks[i]
		if !blocksEqual(ba, bb) {
			return fmt.Errorf("coverage block mismatch: block #%d for %s (%+v mismatches %+v)", i, a.FileName, ba, bb)
		}
	}
	return nil
}
