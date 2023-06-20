package main

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
	"github.com/sashabaranov/go-openai"
)

// Ref: https://platform.openai.com/docs/models
var maxTokens = map[string]int{
	openai.CodexCodeDavinci002: 8001,
	openai.GPT3Dot5Turbo:       4096,
	openai.GPT3Dot5Turbo0301:   4096,
	openai.GPT3TextDavinci002:  4097,
	openai.GPT3TextDavinci003:  4097,
	openai.GPT4:                8192,
	openai.GPT40314:            8192,
	openai.GPT432K:             32768,
	openai.GPT432K0314:         32768,
}

// ref: https://github.com/pkoukk/tiktoken-go#counting-tokens-for-chat-api-calls
func numTokensFromMessages(messages []openai.ChatCompletionMessage, model string) (int, error) {
	tkm, err := tiktoken.EncodingForModel(model)
	if err != nil {
		return 0, fmt.Errorf("EncodingForModel: %v", err)
	}

	var tokens_per_message int
	var tokens_per_name int
	switch model {
	case openai.GPT3Dot5Turbo, openai.GPT3Dot5Turbo0301:
		tokens_per_message = 4
		tokens_per_name = -1
	case openai.GPT4, openai.GPT40314, openai.GPT432K, openai.GPT432K0314:
		tokens_per_message = 3
		tokens_per_name = 1
	default:
		// model not found. Using cl100k_base encoding.
		tokens_per_message = 3
		tokens_per_name = 1
	}

	num_tokens := 0
	for _, message := range messages {
		num_tokens += tokens_per_message
		num_tokens += len(tkm.Encode(message.Content, nil, nil))
		num_tokens += len(tkm.Encode(message.Role, nil, nil))
		num_tokens += len(tkm.Encode(message.Name, nil, nil))
		if message.Name != "" {
			num_tokens += tokens_per_name
		}
	}
	num_tokens += 3

	return num_tokens, nil
}
