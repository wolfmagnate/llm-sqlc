package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go"
	"gopkg.in/yaml.v3"
)

type SQLResponse struct {
	Queries []string `json:"queries"`
}

func GenerateSQL(infraFile string) error {
	// インターフェースの抽出
	ifaceSrc, methods, _, _, err := ExtractFirstInterface(infraFile)
	if err != nil {
		return fmt.Errorf("failed to extract interface: %w", err)
	}
	if len(methods) == 0 {
		return fmt.Errorf("no methods found in the interface from file: %s", infraFile)
	}

	// DBスキーマの読み込み
	schemaPath := filepath.Join("pkg", "infra", "sql", "schema", "schema.sql")
	schemaContentBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		log.Printf("warning: could not read schema file %s: %v", schemaPath, err)
	}
	schemaContent := string(schemaContentBytes)

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

	var allQueries []string
	// 各メソッドごとにSQL生成プロンプトを作成し、クエリを取得する
	for _, method := range methods {
		prompt := fmt.Sprintf(`# Instruction
Please create SQL queries to implement the specified function for the given interface.
We are using sqlc to allow the generated SQL queries to be handled from Golang. Therefore, please ensure that the format of the generated SQL complies with sqlc.

# Function to be implemented
%s

We want to implement %s for this interface.

# Important Notes
You are generating SQL only. There is no need to write the implementation of the function in a programming language.
Please ensure that the SQL queries are optimized for performance and do not cause issues like the N+1 problem.
In the function implementation, processing will be achieved by calling the SQL queries you generate.
It is preferable to have as few queries as possible, but you may use multiple queries if necessary.

# sqlc
The generated queries should include special comments as shown below. Make sure to correctly include the naming, the :one tag (or similar), and the placeholder settings.
We are using PostgreSQL as the DB.
sqlc tries to generate good names for positional parameters, but sometimes it lacks enough context.
Please use @variable_name syntax for the placeholders if possible.

-- name: GetAuthor :one
SELECT * FROM authors
WHERE id = $1 LIMIT 1;

-- name: UpsertAuthorName :one
UPDATE author
SET
  name = CASE WHEN @set_name::bool
    THEN @name::text
    ELSE name
    END
RETURNING *;

-- name: ListAuthorsByIDs :many
SELECT * FROM authors
WHERE id = ANY($1::int[]);

-- name: CreateAuthor :one
INSERT INTO authors (
  name, bio
) VALUES (
  $1, $2
)
RETURNING *;

-- name: UpdateAuthor :exec
UPDATE authors
  SET name = $2,
      bio = $3
WHERE id = $1;

-- name: DeleteAuthor :exec
DELETE FROM authors
WHERE id = $1;

# DB Schema
Below is the schema of the database. Please generate the SQL queries based on this schema:
%s

%s

# Output Format
Output an array named "queries" containing the SQL queries required for the function implementation.
The data type is an array of strings. If necessary, you can output multiple queries.
Each SQL query should start with a comment that is compliant with sqlc.
`, ifaceSrc, method, schemaContent, entityDefinitionsSection)

		resp, err := ChatCompletionHandler[SQLResponse](context.Background(), openai.ChatModelO3Mini, prompt)
		if err != nil {
			return fmt.Errorf("failed to generate SQL queries for method %s: %w", method, err)
		}

		allQueries = append(allQueries, resp.Queries...)
	}

	infraBase := filepath.Join("pkg", "infra")
	infraFileDir := filepath.Dir(infraFile)
	relSubPath, err := filepath.Rel(infraBase, infraFileDir)
	if err != nil {
		relSubPath = ""
	}
	outputDir := filepath.Join("pkg", "infra", "sql", "query", relSubPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	baseName := filepath.Base(infraFile)
	fileNameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	outputFile := filepath.Join(outputDir, fileNameWithoutExt+".sql")
	outputContent := strings.Join(allQueries, "\n\n")
	if err := os.WriteFile(outputFile, []byte(outputContent), 0644); err != nil {
		return fmt.Errorf("failed to write SQL queries to file %s: %w", outputFile, err)
	}

	fmt.Printf("Successfully generated SQL queries and wrote them to %s\n", outputFile)

	sqlcConfigPath := filepath.Join("pkg", "infra", "sqlc.yml")
	configData, err := os.ReadFile(sqlcConfigPath)
	if err != nil {
		log.Printf("warning: could not read sqlc configuration file %s: %v", sqlcConfigPath, err)
		return nil
	}

	var sqlcConfig map[string]interface{}
	if err := yaml.Unmarshal(configData, &sqlcConfig); err != nil {
		log.Printf("warning: failed to parse sqlc configuration file %s: %v", sqlcConfigPath, err)
		return nil
	}

	relativeQueryPath, err := filepath.Rel(infraBase, outputFile)
	if err != nil {
		relativeQueryPath = outputFile
	}

	if sqlBlocks, ok := sqlcConfig["sql"].([]interface{}); ok {
		for _, block := range sqlBlocks {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if queries, ok := blockMap["queries"].([]interface{}); ok {
					found := false
					for _, q := range queries {
						if qs, ok := q.(string); ok && qs == relativeQueryPath {
							found = true
							break
						}
					}
					if !found {
						queries = append(queries, relativeQueryPath)
						blockMap["queries"] = queries
					}
				}
			}
		}
	}

	newConfigData, err := yaml.Marshal(sqlcConfig)
	if err != nil {
		log.Printf("warning: failed to marshal updated sqlc configuration: %v", err)
	} else if err := os.WriteFile(sqlcConfigPath, newConfigData, 0644); err != nil {
		log.Printf("warning: failed to update sqlc configuration file %s: %v", sqlcConfigPath, err)
	} else {
		fmt.Printf("Updated sqlc configuration at %s with new query file: %s\n", sqlcConfigPath, relativeQueryPath)
	}

	return nil
}
