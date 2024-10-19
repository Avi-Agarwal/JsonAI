package server

import (
	"context"
	"fmt"
	"github.com/sashabaranov/go-openai"
)

const (
	model = "gpt-4o"
)

// OpenAIChat Function to chat with OpenAI
func (s Server) OpenAIChat(messages *[]openai.ChatCompletionMessage) (string, error) {
	client := openai.NewClient(s.OpenApiKey)
	response, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:    model,
		Messages: *messages,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get response: %v", err)
	}
	*messages = append(*messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: response.Choices[0].Message.Content})

	return response.Choices[0].Message.Content, nil
}
