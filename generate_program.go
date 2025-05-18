package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openai/openai-go"
	"golang.org/x/tools/imports"
)

type GenerationResponse struct {
	Code       string `json:"code" jsonschema_description:"The code of the implemented function"`
	Import     string `json:"import" jsonschema_description:"The import statements of the function"`
	DocComment string `json:"doccomment" jsonschema_description:"The documentation comment before the function"`
}

// parseGoModFile reads the go.mod file from the project root,
// and extracts the module declaration, Go version, and the direct dependencies
// (ignoring dependencies marked as "// indirect").
func parseGoModFile() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	var moduleLine, goLine string
	var deps []string
	inRequireBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			moduleLine = trimmed
		} else if strings.HasPrefix(trimmed, "go ") {
			goLine = trimmed
		} else if strings.HasPrefix(trimmed, "require (") {
			inRequireBlock = true
		} else if inRequireBlock {
			if trimmed == ")" {
				inRequireBlock = false
			} else {
				// 依存関係行で"// indirect"が含まれていなければ採用
				if trimmed != "" && !strings.Contains(trimmed, "// indirect") {
					deps = append(deps, trimmed)
				}
			}
		} else if strings.HasPrefix(trimmed, "require ") {
			// 単一行の require 文の場合
			if !strings.Contains(trimmed, "// indirect") {
				depLine := strings.TrimPrefix(trimmed, "require ")
				depLine = strings.TrimSpace(depLine)
				deps = append(deps, depLine)
			}
		}
	}

	var builder strings.Builder
	if moduleLine != "" {
		builder.WriteString(moduleLine)
		builder.WriteString("\n\n")
	}
	if goLine != "" {
		builder.WriteString(goLine)
		builder.WriteString("\n\n")
	}
	if len(deps) > 0 {
		builder.WriteString("require (\n")
		for _, dep := range deps {
			builder.WriteString("\t" + dep + "\n")
		}
		builder.WriteString(")\n")
	}
	return builder.String(), nil
}

func GenerateProgram(infraFile string) error {
	// インターフェースとそのメソッド一覧、実装struct定義、実装チェック用の変数定義を抽出する
	ifaceSrc, methods, implStructSrc, varCheckSrc, err := ExtractFirstInterface(infraFile)
	if err != nil {
		return fmt.Errorf("failed to extract interface: %w", err)
	}

	// DB関連のファイル読み込み
	dbFilePath := filepath.Join("pkg", "infra", "db", "db.go")
	dbContent, err := os.ReadFile(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to read db file: %w", err)
	}
	modelsFilePath := filepath.Join("pkg", "infra", "db", "models.go")
	modelsContent, err := os.ReadFile(modelsFilePath)
	if err != nil {
		return fmt.Errorf("failed to read models.go file: %w", err)
	}
	base := filepath.Base(infraFile)
	nameWithoutExt := strings.TrimSuffix(base, ".go")
	sqlFileName := nameWithoutExt + ".sql.go"
	sqlFilePath := filepath.Join("pkg", "infra", "db", sqlFileName)
	sqlContent, err := os.ReadFile(sqlFilePath)
	if err != nil {
		return fmt.Errorf("failed to read sql file %s: %w", sqlFilePath, err)
	}

	// トランザクション処理コードの読み込み
	txFilePath := filepath.Join("pkg", "infra", "txProvider.go")
	txContent, err := os.ReadFile(txFilePath)
	if err != nil {
		return fmt.Errorf("failed to read transaction file: %w", err)
	}

	// エンティティ定義の抽出（存在しなければ警告）
	entities, err := ExtractEntityDefinitions(filepath.Join("pkg", "domain", "entity"))
	if err != nil {
		log.Printf("warning: could not extract entity definitions: %v", err)
	}
	var entityDefBuilder strings.Builder
	entityDefBuilder.WriteString("# Entity Definition\nThe function we are implementing references the following Entity. Here are the type definitions and the definition of the New function for generating the Entity:\n")
	for _, entity := range entities {
		relPath, err := filepath.Rel(".", entity.FileName)
		if err != nil {
			relPath = entity.FileName
		}
		entityDefBuilder.WriteString(fmt.Sprintf("## %s\n", relPath))
		entityDefBuilder.WriteString("```\n")
		entityDefBuilder.WriteString(entity.Code)
		entityDefBuilder.WriteString("\n```\n")
	}
	entityDefinitionsSection := entityDefBuilder.String()

	// 実装ガイドライン
	implGuidelines := `## Implementation Guidelines
- Always create the Entity using the New function. Do not instantiate the struct directly.
- For queries that retrieve a single record by ID, first check the cache, and if it is not found, then issue a DB query.
- The cache key should be in the format "EntityType:EntityID".
- If the method argument is an entity type (for example, id entity.ChannelID), then if the corresponding record does not exist in the DB, return an error.
- If the method argument is a basic data type (for example, id string), then if the corresponding record does not exist in the DB, return nil or an empty slice rather than an error.

## Cache
The infrastructure implementation uses a cache to speed up access by avoiding direct DB queries.
The cache is defined in pkg/infra/cache.go as follows:

package infra

import "time"

type Cache interface {
	Set(k string, x interface{}, d time.Duration)
	Get(k string) (interface{}, bool)
}

## Implementation Pattern
query := db.New(tx)
// Use cache if necessary. In some cases, caching may not be used.
cacheKey := fmt.Sprintf("EntityType:%d", id)
if cachedEntity, found := repo.Cache.Get(cacheKey); found {
    // If the cache contains the entity, return it.
}

// Call the DB query via its function
// For example: query.GetSomething(ctx)

// Convert the retrieved data to an Entity using the New function.

// If needed, store the entity in the cache. Set the cache duration appropriately.
repo.Cache.Set(cacheKey, entity, cacheDuration)`

	// プロジェクトルートの go.mod から直接依存関係のみ抽出
	goModContent, err := parseGoModFile()
	if err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}

	// 各メソッドの実装生成結果を格納するスライス
	var generatedMethods []*GenerationResponse
	// 各メソッドのimport文をまとめるためのスライス
	var allMethodImports []string

	// infraFileのディレクトリから、ルートからの相対パスを取得（例: pkg/infra/subdir）
	relDir, err := filepath.Rel(".", filepath.Dir(infraFile))
	if err != nil {
		relDir = filepath.Dir(infraFile)
	}

	// 各メソッドごとに生成プロンプトを作成し、実装コードを取得する
	for _, methodName := range methods {
		var promptBuilder strings.Builder
		promptBuilder.WriteString("# Instruction\n")
		promptBuilder.WriteString("Please implement the function as specified with golang.\n\n")
		promptBuilder.WriteString("# Function to Implement\n")
		promptBuilder.WriteString(fmt.Sprintf("Implement the %s function of the interface defined below.\n\n", methodName))
		promptBuilder.WriteString("Interface definition:\n")
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(ifaceSrc)
		promptBuilder.WriteString("\n```\n\n")
		promptBuilder.WriteString("Implementation struct definition:\n")
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(implStructSrc)
		promptBuilder.WriteString("\n\n")
		promptBuilder.WriteString(varCheckSrc)
		promptBuilder.WriteString("\n```\n")
		promptBuilder.WriteString("# DB\n")
		promptBuilder.WriteString("You will communicate with the database using the code provided below.\n")
		promptBuilder.WriteString("## pkg/infra/db/db.go\n")
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(string(dbContent))
		promptBuilder.WriteString("\n```\n")
		promptBuilder.WriteString("## pkg/infra/db/models.go\n")
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(string(modelsContent))
		promptBuilder.WriteString("\n```\n")
		promptBuilder.WriteString(fmt.Sprintf("## pkg/infra/db/%s\n", sqlFileName))
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(string(sqlContent))
		promptBuilder.WriteString("\n```\n")
		promptBuilder.WriteString(entityDefinitionsSection)
		promptBuilder.WriteString("\n")
		promptBuilder.WriteString("# Transactions\n")
		promptBuilder.WriteString(string(txContent))
		promptBuilder.WriteString("\n\n")
		promptBuilder.WriteString(implGuidelines)
		promptBuilder.WriteString("\n\n")
		promptBuilder.WriteString("# Output Schema\n")
		promptBuilder.WriteString("Define the JSON schema for the output with the following properties:\n")
		promptBuilder.WriteString("- code (string): The code of the implemented function. It starts from func keyword. Don't write any import statement. Only the code of a function.\n")
		promptBuilder.WriteString("- import (string): The import statements of the function. It starts from `import (` and ends with `)`\n")
		promptBuilder.WriteString("- doccomment (string): The documentation comment before the function.\n")
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(goModContent)
		promptBuilder.WriteString("```\n")
		promptBuilder.WriteString(fmt.Sprintf("Your implementation is in root/%s package.\n", relDir))
		promptBuilder.WriteString("# Directory Structure\n")
		promptBuilder.WriteString("entity is in root/pkg/domain/entity package.\n")
		promptBuilder.WriteString("db is in root/pkg/infra/db package.\n")
		promptBuilder.WriteString("Your implementation file is provided as an argument and may reside in a subdirectory of pkg/infra.\n")

		promptText := promptBuilder.String()

		response, err := ChatCompletionHandler[GenerationResponse](context.Background(), openai.ChatModelO3Mini, promptText)
		if err != nil {
			return fmt.Errorf("ChatCompletionHandler error for method %s: %w", methodName, err)
		}

		// 生成結果を保存
		generatedMethods = append(generatedMethods, response)

		// 各メソッドのインポート文を収集する
		impBlock := strings.TrimSpace(response.Import)
		impBlock = strings.TrimPrefix(impBlock, "import (")
		impBlock = strings.TrimSuffix(impBlock, ")")
		lines := strings.Split(impBlock, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" {
				allMethodImports = append(allMethodImports, trimmedLine)
			}
		}
	}

	// 重複除去とアルファベット順のソート（標準ライブラリ sort を利用）
	importMap := make(map[string]struct{})
	for _, imp := range allMethodImports {
		importMap[imp] = struct{}{}
	}
	var importList []string
	for imp := range importMap {
		importList = append(importList, imp)
	}
	sort.Strings(importList)

	var finalImportBuilder strings.Builder
	finalImportBuilder.WriteString("import (\n")
	for _, imp := range importList {
		finalImportBuilder.WriteString("\t" + imp + "\n")
	}
	finalImportBuilder.WriteString(")\n")
	finalImportBlock := finalImportBuilder.String()

	// 最終的なコード生成：
	// ・package文（ファイルの直上のディレクトリ名をパッケージ名とする）
	// ・整形済みimportブロック
	// ・インターフェース定義
	// ・実装するstructの定義
	// ・varによる実装チェック
	// ・生成された各関数（doccommentとコード）を順に並べる
	var finalCodeBuilder strings.Builder
	pkgName := filepath.Base(filepath.Dir(infraFile))
	finalCodeBuilder.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
	finalCodeBuilder.WriteString(finalImportBlock)
	finalCodeBuilder.WriteString("\n")
	finalCodeBuilder.WriteString(ifaceSrc)
	finalCodeBuilder.WriteString("\n\n")
	finalCodeBuilder.WriteString(implStructSrc)
	finalCodeBuilder.WriteString("\n\n")
	finalCodeBuilder.WriteString(varCheckSrc)
	finalCodeBuilder.WriteString("\n\n")
	for _, method := range generatedMethods {
		if strings.TrimSpace(method.DocComment) != "" {
			finalCodeBuilder.WriteString(method.DocComment)
			finalCodeBuilder.WriteString("\n")
		}
		finalCodeBuilder.WriteString(method.Code)
		finalCodeBuilder.WriteString("\n\n")
	}

	// 生成されたコードを一旦バイトスライスに変換
	finalCode := []byte(finalCodeBuilder.String())

	// VSCode保存時と同様の自動import整形処理をgolang.org/x/tools/importsで実行
	formattedCode, err := imports.Process(infraFile, finalCode, nil)
	if err != nil {
		return fmt.Errorf("failed to process imports: %w", err)
	}

	// infraFileの内容を上書きする
	if err := os.WriteFile(infraFile, formattedCode, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", infraFile, err)
	}

	log.Printf("Successfully updated %s", infraFile)
	return nil
}
