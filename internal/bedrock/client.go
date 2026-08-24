package bedrock

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type Model struct {
	ID   string
	Name string
}

type modelsAPI interface {
	ListFoundationModels(context.Context, *bedrock.ListFoundationModelsInput, ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)
}

type AWSModelsClient struct{ client modelsAPI }

func NewAWSModelsClient(cfg aws.Config) *AWSModelsClient {
	return &AWSModelsClient{client: bedrock.NewFromConfig(cfg)}
}

func (c *AWSModelsClient) ListModels(ctx context.Context) ([]Model, error) {
	result, err := c.client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(result.ModelSummaries))
	for _, model := range result.ModelSummaries {
		if model.ModelId == nil || model.ModelName == nil || !supportsConverse(*model.ModelId, model.InferenceTypesSupported) {
			continue
		}
		models = append(models, Model{ID: *model.ModelId, Name: *model.ModelName})
	}
	sort.Slice(models, func(i, j int) bool { return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name) })
	return models, nil
}

func supportsConverse(modelID string, inferenceTypes []bedrocktypes.InferenceType) bool {
	if !supportsOnDemand(inferenceTypes) {
		return false
	}
	modelID = strings.ToLower(modelID)
	for _, prefix := range []string{
		"ai21.jamba",
		"amazon.nova",
		"amazon.titan-text",
		"anthropic.claude-3",
		"anthropic.claude-3-5",
		"anthropic.claude-3-7",
		"anthropic.claude-4",
		"anthropic.claude-haiku-4",
		"anthropic.claude-opus-4",
		"anthropic.claude-sonnet-4",
		"cohere.command-r",
		"cohere.command-r-plus",
		"meta.llama3",
		"meta.llama3-1",
		"meta.llama3-2",
		"meta.llama3-3",
		"mistral.mistral-large",
		"mistral.mistral-small",
		"mistral.mixtral",
	} {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func supportsOnDemand(inferenceTypes []bedrocktypes.InferenceType) bool {
	for _, inferenceType := range inferenceTypes {
		if inferenceType == bedrocktypes.InferenceTypeOnDemand {
			return true
		}
	}
	return false
}

type ChatClient interface {
	Converse(context.Context, string, []Message) (string, error)
}

type Message struct {
	Role    string
	Content string
}

type runtimeAPI interface {
	Converse(context.Context, *bedrockruntime.ConverseInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

type AWSChatClient struct{ client runtimeAPI }

func NewAWSChatClient(cfg aws.Config) *AWSChatClient {
	return &AWSChatClient{client: bedrockruntime.NewFromConfig(cfg)}
}

func (c *AWSChatClient) Converse(ctx context.Context, modelID string, messages []Message) (string, error) {
	input := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		input = append(input, types.Message{
			Role:    types.ConversationRole(message.Role),
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: message.Content}},
		})
	}
	result, err := c.client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId:  aws.String(modelID),
		Messages: input,
	})
	if err != nil {
		return "", err
	}
	if result == nil || result.Output == nil {
		return "", fmt.Errorf("Bedrock returned an empty response")
	}
	message, ok := result.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("Bedrock returned an unsupported response")
	}
	var response strings.Builder
	for _, block := range message.Value.Content {
		if text, ok := block.(*types.ContentBlockMemberText); ok {
			response.WriteString(text.Value)
		}
	}
	if response.Len() == 0 {
		return "", fmt.Errorf("Bedrock returned no text")
	}
	return response.String(), nil
}
