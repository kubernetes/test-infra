package git

import (
	"os/exec"
	"strings"
)

const shortShaLength = 7

func CurrentCommitHash() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func Shorten(sha string) string {
	return sha[:shortShaLength]
}
