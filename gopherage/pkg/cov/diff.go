package cov

import (
	"errors"
	"fmt"
	"golang.org/x/tools/cover"
)

// DiffProfiles returns the difference between two sets of coverage profiles.
// The profiles are expected to be from a single execution of the same binary
// (or multiple binaries, if using a merged coverage profile)
func DiffProfiles(before []*cover.Profile, after []*cover.Profile) ([]*cover.Profile, error) {
	var diff []*cover.Profile
	if len(before) != len(after) {
		return nil, errors.New("before and after have different numbers of profiles")
	}
	for i, beforeProfile := range before {
		afterProfile := after[i]
		if beforeProfile.FileName != afterProfile.FileName {
			return nil, errors.New("coverage filename mismatch")
		}
		if len(beforeProfile.Blocks) != len(afterProfile.Blocks) {
			return nil, fmt.Errorf("coverage for %s has differing block count", beforeProfile.FileName)
		}
		diffProfile := cover.Profile{FileName: beforeProfile.FileName, Mode: beforeProfile.Mode}
		for j, beforeBlock := range beforeProfile.Blocks {
			afterBlock := afterProfile.Blocks[j]
			if !blocksEqual(beforeBlock, afterBlock) {
				return nil, errors.New("coverage block mismatch")
			}
			diffBlock := cover.ProfileBlock{
				StartLine: beforeBlock.StartLine,
				StartCol:  beforeBlock.StartCol,
				EndLine:   beforeBlock.EndLine,
				EndCol:    beforeBlock.EndCol,
				NumStmt:   beforeBlock.NumStmt,
				Count:     afterBlock.Count - beforeBlock.Count,
			}
			diffProfile.Blocks = append(diffProfile.Blocks, diffBlock)
		}
		diff = append(diff, &diffProfile)
	}
	return diff, nil
}
