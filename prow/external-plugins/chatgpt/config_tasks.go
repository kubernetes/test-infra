package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultSystemMessage          = "You are an experienced software developer. You will act as a reviewer for a GitHub Pull Request, and you should answer by markdown format."
	defaultPromte                 = "Please identify potential problems and give some fixing suggestions."
	defaultPrPatchIntroducePromte = "This is the diff for the pull request:"
	defaultMaxResponseTokens      = 500
	defaultTemperature            = 0.7
	defaultStaticOutHeadnote      = `> **I have already done a preliminary review for you, and I hope to help you do a better job.**
------
`
)

// TasksConfig represent the all tasks store for the plugin.
//
// layer: org|repo / task-name / task-config
type TasksConfig map[string]RepoTasks

// RepoTasks represent the tasks for a repo or ORG.
type RepoTasks map[string]*Task

// Task reprensent the config for AI task item.
//
// $SystemMessage
// --------------
// < $Prompt
// < Here are the serval context contents:
// $ExternalContexts.each do
//
//	< - format(it.PromptTpl, fetch(it.ResURL))
//
// < $PatchIntroducerPrompt
// < ```diff
// < diff content
// < ```
// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~
// > <OutputStaticHeadNote>
// > responses from AI server.
//
// TODO(wuhuizuo): using go template to comose the question.
type Task struct {
	Description          string             `yaml:"description,omitempty" json:"description,omitempty"`
	SystemMessage        string             `yaml:"system_message,omitempty" json:"system_message,omitempty"`
	UserPrompt           string             `yaml:"user_prompt,omitempty" json:"user_prompt,omitempty"`
	PatchIntroducePrompt string             `yaml:"patch_introduce_prompt,omitempty" json:"patch_introduce_prompt,omitempty"`
	OutputStaticHeadNote string             `yaml:"output_static_head_note,omitempty" json:"output_static_head_note,omitempty"`
	MaxResponseTokens    int                `yaml:"max_response_tokens,omitempty" json:"max_response_tokens,omitempty"`
	ExternalContexts     []*ExternalContext `yaml:"external_contexts,omitempty" json:"external_contexts,omitempty"`
}

type ExternalContext struct {
	PromptTpl  string `yaml:"prompt_tpl,omitempty" json:"prompt_tpl,omitempty"`
	ResURL     string `yaml:"res_url,omitempty" json:"res_url,omitempty"`
	resContent []byte //nolint: unused // to be implemented.
}

func (ec *ExternalContext) Content() ([]byte, error) {
	return nil, errors.New("not implemented")
}

// TaskAgent agent for fetch tasks with watching and hot reload.
type TaskAgent struct {
	ConfigAgent[TasksConfig]
}

// NewTaskAgent returns a new ConfigLoader.
func NewTaskAgent(path string, watchInterval time.Duration) (*TaskAgent, error) {
	c := &TaskAgent{ConfigAgent: ConfigAgent[TasksConfig]{path: path}}
	err := c.Reload(path)
	if err != nil {
		return nil, err
	}

	go c.WatchConfig(context.Background(), watchInterval, c.Reload)

	return c, nil
}

// Get return the config data.
func (c *TaskAgent) TasksFor(org, repo string) (map[string]*Task, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fullName := fmt.Sprintf("%s/%s", org, repo)
	repoTasks, ok := c.Data()[fullName]
	if ok {
		return repoTasks, nil
	}
	logrus.Debugf("no tasks for repo: %s", fullName)

	orgTasks, ok := c.config[org]
	if ok {
		return orgTasks, nil
	}
	logrus.Debugf("no tasks for org %s", org)
	logrus.Debugf("all tasks: %#v", c.config)
	return nil, nil
}

// Task return the given task config.
func (c *TaskAgent) Task(org, repo, taskName string, needDefault bool) (*Task, error) {
	tasks, err := c.TasksFor(org, repo)
	if err != nil {
		return nil, err
	}

	task := tasks[taskName]
	if task != nil {
		return task, nil
	}

	if needDefault {
		return c.DefaultTask(), nil
	}

	return nil, nil
}

func (c *TaskAgent) DefaultTask() *Task {
	return &Task{
		SystemMessage:        defaultSystemMessage,
		UserPrompt:           defaultPromte,
		MaxResponseTokens:    defaultMaxResponseTokens,
		PatchIntroducePrompt: defaultPrPatchIntroducePromte,
	}
}
