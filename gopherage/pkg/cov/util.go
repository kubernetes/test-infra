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

// copyProfile returns a deep copy of profile.
func copyProfile(profile cover.Profile) cover.Profile {
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
