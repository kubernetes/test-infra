package updatebot

import (
	"testing"

	"github.com/go-git/go-git/v5/config"
	"github.com/sirupsen/logrus"
)

func TestUpdateChangelog(t *testing.T) {
	context := &UpdateContext{
		MainRepo:   "dtk",
		OwnerLogin: "peeweep-test",
		UpdateHead: "topic-update",
		UpdateBase: "master",
		UpdateToVersion: "6.0.2",
	}
	err := UpdateChangelog(logrus.NewEntry(logrus.New()), &config.Submodule{
		Name:   "dtkcommon",
		Path:   "dtkcommon",
		URL:    "https://github.com/peeweep-test/dtkcommon",
		Branch: "master",
	}, context)
	if err != nil {
		t.Error(err)
	}
}
