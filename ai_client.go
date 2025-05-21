package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// AIClient wraps the OpenAI client.
type AIClient struct {
	client *openai.Client
}

// NewAIClient creates a new AIClient.
// It loads environment variables, retrieves the API key, and initializes the OpenAI client.
func NewAIClient() (*AIClient, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println(".env ファイルの読み込みに失敗しましたが、環境変数を使用して続行します")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set in .env or environment")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &AIClient{client: client}, nil
}

// SchemaGenerator is a utility function to generate JSON schemas from Go types.
// It remains a package-level function as it does not depend on AIClient's state.
func SchemaGenerator[T any]() interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	return reflector.Reflect(v)
}

// ChatCompletionHandler is a method of AIClient that uses the JSON schema
// to process completions with the OpenAI API.
func (ac *AIClient) ChatCompletionHandler[T any](ctx context.Context, model string, prompt string) (*T, error) {
	schema := SchemaGenerator[T]()

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        openai.F("response_schema"),
		Description: openai.F("Structured response based on JSON schema"),
		Schema:      openai.F(schema),
		Strict:      openai.Bool(true),
	}

	chat, err := ac.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		}),
		ResponseFormat: openai.F[openai.ChatCompletionNewParamsResponseFormatUnion](
			openai.ResponseFormatJSONSchemaParam{
				Type:       openai.F(openai.ResponseFormatJSONSchemaTypeJSONSchema),
				JSONSchema: openai.F(schemaParam),
			},
		),
		Model: openai.F(model),
	})
	if err != nil {
		return nil, err
	}

	fmt.Println(prompt) // For debugging, consider removing or making conditional
	fmt.Println(chat.Choices[0].Message.Content) // For debugging

	var result T
	err = json.Unmarshal([]byte(chat.Choices[0].Message.Content), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
