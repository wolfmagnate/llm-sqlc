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

var client *openai.Client

// --- 初期化処理 ---
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println(".env ファイルの読み込みに失敗しましたが、環境変数を使用して続行します")
	}

	// 環境変数からAPIキーを取得
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY not set in .env")
	}

	// OpenAIクライアントの初期化
	client = openai.NewClient(option.WithAPIKey(apiKey))
}

// SchemaGenerator は任意の構造体からJSONスキーマを生成します
func SchemaGenerator[T any]() interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	return reflector.Reflect(v)
}

// ChatCompletionHandler はJSONスキーマを使ってOpenAI APIの補完を処理します
func ChatCompletionHandler[T any](ctx context.Context, model string, prompt string) (*T, error) {
	// JSONスキーマ生成
	schema := SchemaGenerator[T]()

	// スキーマパラメータ設定
	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        openai.F("response_schema"),
		Description: openai.F("Structured response based on JSON schema"),
		Schema:      openai.F(schema),
		Strict:      openai.Bool(true),
	}

	// OpenAI APIを呼び出し
	chat, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
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

	fmt.Println(prompt)
	fmt.Println(chat.Choices[0].Message.Content)

	// 応答を構造体にデコード
	var result T
	err = json.Unmarshal([]byte(chat.Choices[0].Message.Content), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
