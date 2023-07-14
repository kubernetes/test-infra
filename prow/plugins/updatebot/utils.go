package updatebot

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

type Submodule struct {
	Name   string
	Path   string
	URL    string
	Branch string
}

func ParseDotGitmodulesContent(content []byte) ([]Submodule, error) {
	file, err := ini.Load(content)
	if err != nil {
		return nil, err
	}
	sections := file.Sections()
	var result []Submodule
	for _, section := range sections {
		rg := regexp.MustCompile(`submodule\s"(.*)"`)
		match := rg.FindStringSubmatch(section.Name())
		if len(match) != 2 {
			continue
		}
		entry := Submodule{
			Name:   match[1],
			Path:   section.Key("path").String(),
			URL:    section.Key("url").String(),
			Branch: section.Key("branch").String(),
		}
		result = append(result, entry)
	}
	return result, nil
}

func UpdateChangelog(entry *logrus.Entry, submodule *config.Submodule, context *Session) error {
	cwd, err := os.MkdirTemp("", "updatebot*")
	defer os.RemoveAll(cwd)
	if err != nil {
		entry.WithError(err).Warn("Cannot create a work directory")
		return err
	}
	repo, err := git.PlainClone(cwd, false, &git.CloneOptions{
		URL:           submodule.URL,
		ReferenceName: plumbing.NewBranchReferenceName(submodule.Branch),
	})
	if err != nil {
		entry.WithError(err).Warn("Cannot clone normally")
		return err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		entry.WithError(err).Warn("Cannot get worktree")
		return err
	}
	base, err := repo.Head()
	updateBranch := plumbing.NewBranchReferenceName(context.UpdateHeadBranch)
	if err != nil {
		entry.WithError(err).Warn("Cannot get update base branch")
		return err
	}
	worktree.Checkout(&git.CheckoutOptions{
		Hash:   base.Hash(),
		Branch: updateBranch,
		Create: true,
		Force:  true,
	})
	cmd := exec.Cmd{
		Path: "gbp",
		Dir: cwd,
		Args: []string{
			"deepin-changelog",
			"-N",
			context.UpdateToVersion,
			"--spawn-editor=never",
			"--distribution=unstable",
			"--force-distribution",
			"--git-author",
			"--ignore-branch",
		},
	}
	err = cmd.Run()
	if err != nil {
		logrus.WithError(err).Warn("Execute gbp failed")
		return err
	}
	commitMessage := fmt.Sprintf("chore: update changelog\n\nRelease %s.", context.UpdateToVersion)
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		All: true,
	})
	if err != nil {
		logrus.WithError(err).Warn("Cannot commit")
		return err
	}
	refSpec := config.RefSpec(fmt.Sprintf("+%s:%s", updateBranch, updateBranch))
	err = repo.Push(&git.PushOptions{
		Force: true,
		RefSpecs: []config.RefSpec{refSpec},
	})
	if err != nil {
		logrus.WithError(err).Warn("Cannot push to remote")
		return err
	}
	return nil
}
