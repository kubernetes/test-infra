package main

import (
	"context"
	"time"

	"github.com/sashabaranov/go-openai"
)

type OpenaiConfig struct {
	Token      string `yaml:"token,omitempty" json:"token,omitempty"`
	BaseURL    string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	OrgID      string `yaml:"org_id,omitempty" json:"org_id,omitempty"`
	APIType    string `yaml:"api_type,omitempty" json:"api_type,omitempty"`       // OPEN_AI | AZURE | AZURE_AD
	APIVersion string `yaml:"api_version,omitempty" json:"api_version,omitempty"` // 2023-03-15-preview, required when APIType is APITypeAzure or APITypeAzureAD
	Engine     string `yaml:"engine,omitempty" json:"engine,omitempty"`           // required when APIType is APITypeAzure or APITypeAzureAD, it's the deploy instance name.
	Model      string `yaml:"model,omitempty" json:"model,omitempty"`             // OpenAI models, list ref: https://github.com/sashabaranov/go-openai/blob/master/completion.go#L15-L38

	client *openai.Client
}

func (cfg *OpenaiConfig) initClient() {
	if cfg.client == nil {
		openaiCfg := openai.DefaultConfig(cfg.Token)
		openaiCfg.BaseURL = cfg.BaseURL
		openaiCfg.OrgID = cfg.OrgID
		openaiCfg.APIType = openai.APIType(cfg.APIType)
		openaiCfg.APIVersion = cfg.APIVersion
		openaiCfg.Engine = cfg.Engine

		cfg.client = openai.NewClientWithConfig(openaiCfg)
	}
}

// OpenaiAgent agent for openai clients with watching and hot reload.
type OpenaiAgent struct {
	ConfigAgent[OpenaiConfig]
}

// NewOpenaiAgent returns a new openai loader.
func NewOpenaiAgent(path string, watchInterval time.Duration) (*OpenaiAgent, error) {
	c := &OpenaiAgent{ConfigAgent: ConfigAgent[OpenaiConfig]{path: path}}
	err := c.Reload(path)
	if err != nil {
		return nil, err
	}

	go c.WatchConfig(context.Background(), watchInterval, c.Reload)

	return c, nil
}

func (a *OpenaiAgent) Reload(file string) error {
	if err := a.ConfigAgent.Reload(file); err != nil {
		return err
	}

	a.mu.Lock()
	a.config.initClient()
	a.mu.Unlock()

	return nil
}

type OpenaiWrapAgent struct {
	small              *OpenaiAgent
	large              *OpenaiAgent
	largeDownThreshold int
}

// NewOpenaiAgent returns a new openai loader.
func NewWrapOpenaiAgent(defaultPath, largePath string, largeDownThreshold int, watchInterval time.Duration) (*OpenaiWrapAgent, error) {
	d, err := NewOpenaiAgent(defaultPath, watchInterval)
	if err != nil {
		return nil, err
	}

	ret := &OpenaiWrapAgent{small: d, largeDownThreshold: largeDownThreshold}
	if largePath != "" {
		l, err := NewOpenaiAgent(largePath, watchInterval)
		if err != nil {
			return nil, err
		}

		ret.large = l
	}

	return ret, nil
}

func (a *OpenaiWrapAgent) ClientFor(msgLen int) (*openai.Client, string) {
	if a.large != nil &&
		a.largeDownThreshold > 0 &&
		msgLen > a.largeDownThreshold {
		return a.large.Data().client, a.large.Data().Model
	}

	return a.small.Data().client, a.small.Data().Model
}
