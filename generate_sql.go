package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SQLGenerator handles the generation of SQL files.
type SQLGenerator struct {
	aiClient *AIClient
}

// NewSQLGenerator creates a new instance of SQLGenerator.
func NewSQLGenerator(aiClient *AIClient) *SQLGenerator {
	return &SQLGenerator{aiClient: aiClient}
}

type SQLResponse struct {
	Queries []string `json:"queries"`
}

// generateSQLLogic contains the core logic of generating the SQL.
// This will be broken down into smaller methods.
func (sg *SQLGenerator) generateSQLLogic(infraFile string) error {
	// インターフェースの抽出
	ifaceSrc, methods, err := sg.extractInterfaceData(infraFile)
	if err != nil {
		return fmt.Errorf("failed to extract interface data: %w", err) // Error message updated for clarity
	}

	// DBスキーマの読み込み
	schemaContent, err := sg.loadDBSchema()
	if err != nil {
		// The original code logs a warning and continues, so we replicate that.
		log.Printf("warning: could not load DB schema: %v", err)
		// schemaContent will be empty, and the prompt generation will handle it.
	}

	// エンティティ定義の抽出（存在しなければ警告）
	entityDefinitionsSection, err := sg.loadEntityDefinitions()
	if err != nil {
		// The original code logs a warning and continues.
		log.Printf("warning: could not load entity definitions: %v", err)
		// entityDefinitionsSection will be empty, and prompt generation will handle it.
	}

	var allQueries []string
	// 各メソッドごとにSQL生成プロンプトを作成し、クエリを取得する
	for _, method := range methods {
		prompt := sg.preparePromptForMethod(method, ifaceSrc, schemaContent, entityDefinitionsSection)
		resp, err := sg.generateSQLForMethod(prompt)
		if err != nil {
			return fmt.Errorf("failed to generate SQL queries for method %s: %w", method, err)
		}

		allQueries = append(allQueries, resp.Queries...)
	}

	outputFile, err := sg.writeSQLFile(infraFile, allQueries)
	if err != nil {
		return fmt.Errorf("failed to write SQL file: %w", err)
	}
	fmt.Printf("Successfully generated SQL queries and wrote them to %s\n", outputFile)

	infraBase := filepath.Join("pkg", "infra") // Define infraBase for use in updateSqlcConfig
	err = sg.updateSqlcConfig(outputFile, infraBase)
	if err != nil {
		// Log warning as original behavior for sqlc.yml update issues
		log.Printf("warning: failed to update sqlc.yml: %v", err)
	}

	return nil
}

// Generate is the main public method that orchestrates the SQL generation process.
func (sg *SQLGenerator) Generate(infraFile string) error {
	return sg.generateSQLLogic(infraFile)
}

// extractInterfaceData wraps the call to ExtractFirstInterface and checks for methods.
func (sg *SQLGenerator) extractInterfaceData(infraFile string) (ifaceSrc string, methods []string, err error) {
	ifaceSrc, methods, _, _, err = ExtractFirstInterface(infraFile)
	if err != nil {
		return "", nil, fmt.Errorf("failed to extract interface from %s: %w", infraFile, err)
	}
	if len(methods) == 0 {
		return "", nil, fmt.Errorf("no methods found in the interface from file: %s", infraFile)
	}
	return ifaceSrc, methods, nil
}

// loadDBSchema reads the schema.sql file.
func (sg *SQLGenerator) loadDBSchema() (schemaContent string, err error) {
	schemaPath := filepath.Join("pkg", "infra", "sql", "schema", "schema.sql")
	schemaContentBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		// Return the error to allow the caller to decide on logging/handling
		return "", fmt.Errorf("could not read schema file %s: %w", schemaPath, err)
	}
	return string(schemaContentBytes), nil
}

// loadEntityDefinitions loads and formats entity definitions.
func (sg *SQLGenerator) loadEntityDefinitions() (entityDefinitionsSection string, err error) {
	entities, err := ExtractEntityDefinitions(filepath.Join("pkg", "domain", "entity"))
	if err != nil {
		return "", fmt.Errorf("could not extract entity definitions: %w", err)
	}
	var entityDefBuilder strings.Builder
	entityDefBuilder.WriteString("# Entity Definition\nThe function we are implementing references the following Entity. Here are the type definitions and the definition of the New function for generating the Entity:\n")
	for _, entity := range entities {
		relPath, relErr := filepath.Rel(".", entity.FileName)
		if relErr != nil {
			relPath = entity.FileName // Fallback to full path if Rel fails
		}
		entityDefBuilder.WriteString(fmt.Sprintf("## %s\n", relPath))
		entityDefBuilder.WriteString("```\n")
		entityDefBuilder.WriteString(entity.Code)
		entityDefBuilder.WriteString("\n```\n")
	}
	return entityDefBuilder.String(), nil
}

// preparePromptForMethod constructs the prompt for a single method.
func (sg *SQLGenerator) preparePromptForMethod(methodName, ifaceSrc, schemaContent, entityDefsStr string) string {
	return fmt.Sprintf(`# Instruction
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
`, ifaceSrc, methodName, schemaContent, entityDefsStr)
}

// generateSQLForMethod calls the ChatCompletionHandler for SQL generation.
func (sg *SQLGenerator) generateSQLForMethod(prompt string) (*SQLResponse, error) {
	return sg.aiClient.ChatCompletionHandler[SQLResponse](context.Background(), "gpt-4.1-mini", prompt)
}

// writeSQLFile determines the output path, creates directories, and writes the queries.
func (sg *SQLGenerator) writeSQLFile(infraFile string, allQueries []string) (outputFilePath string, err error) {
	infraBase := filepath.Join("pkg", "infra")
	infraFileDir := filepath.Dir(infraFile)
	relSubPath, err := filepath.Rel(infraBase, infraFileDir)
	if err != nil {
		// If infraFile is not under infraBase, relSubPath might be complex or error.
		// For simplicity, using empty, meaning it will go into "pkg/infra/sql/query/.sql"
		// This matches original behavior if Rel errors.
		relSubPath = ""
	}
	outputDir := filepath.Join("pkg", "infra", "sql", "query", relSubPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	baseName := filepath.Base(infraFile)
	fileNameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	outputFilePath = filepath.Join(outputDir, fileNameWithoutExt+".sql")
	outputContent := strings.Join(allQueries, "\n\n")
	if err := os.WriteFile(outputFilePath, []byte(outputContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write SQL queries to file %s: %w", outputFilePath, err)
	}
	return outputFilePath, nil
}

// updateSqlcConfig handles reading, updating, and writing the sqlc.yml configuration.
func (sg *SQLGenerator) updateSqlcConfig(sqlFilePath string, infraFileBasePath string) error {
	sqlcConfigPath := filepath.Join(infraFileBasePath, "sqlc.yml") // Construct path using infraFileBasePath
	configData, err := os.ReadFile(sqlcConfigPath)
	if err != nil {
		// Return error to be logged by the caller, consistent with original behavior of logging warnings.
		return fmt.Errorf("could not read sqlc configuration file %s: %w", sqlcConfigPath, err)
	}

	var sqlcConfig map[string]interface{}
	if err := yaml.Unmarshal(configData, &sqlcConfig); err != nil {
		return fmt.Errorf("failed to parse sqlc configuration file %s: %w", sqlcConfigPath, err)
	}

	relativeQueryPath, err := filepath.Rel(infraFileBasePath, sqlFilePath)
	if err != nil {
		// If Rel fails, use the original sqlFilePath (less ideal, but better than erroring out here)
		relativeQueryPath = sqlFilePath
	}

	// Navigate through the YAML structure to update the queries list
	sqlBlocks, ok := sqlcConfig["sql"].([]interface{})
	if !ok {
		// If "sql" key doesn't exist or is not a slice, we can't proceed.
		// This case might indicate a malformed sqlc.yml or a structure we don't handle.
		// For now, return an error or log a warning. The original code didn't explicitly handle this.
		return fmt.Errorf("sqlc.yml does not contain a valid 'sql' block as an array")
	}

	foundQueryInAnyBlock := false
	for _, block := range sqlBlocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue // Skip if block is not a map
		}

		queries, ok := blockMap["queries"].([]interface{})
		if !ok {
			// If 'queries' is not a []interface{}, this block might not be what we expect.
			// Create it if it's missing under a specific schema/gen setup? For now, skip.
			// Or, if it's a string, convert to []interface{}. The original code assumes it's []interface{}.
			// Let's assume for now that if 'queries' exists, it's a list.
			// If it doesn't exist in a block where it should, that's a different issue.
			// The original code would effectively skip this block if 'queries' wasn't a []interface{}.
			continue
		}

		currentBlockFoundQuery := false
		for _, q := range queries {
			if qs, ok := q.(string); ok && qs == relativeQueryPath {
				currentBlockFoundQuery = true
				foundQueryInAnyBlock = true
				break
			}
		}

		if !currentBlockFoundQuery {
			// Add the query path to this block if not found.
			// The original logic adds to the first block where it's not found.
			// This might not be ideal if there are multiple 'sql' blocks with different 'queries' lists.
			// However, typical sqlc.yml has one main 'queries' list under a 'gen' block.
			// For now, mimic the original behavior: add to any list that doesn't have it.
			// A more robust solution might target a specific block based on schema/gen settings.
			queries = append(queries, relativeQueryPath)
			blockMap["queries"] = queries
			foundQueryInAnyBlock = true // Mark that we've added it
		}
	}
	
	// If the query path was not found in any existing query list (and thus not added),
	// this might mean there's no suitable block. This part of the logic is tricky
	// and depends on the expected structure of sqlc.yml. The original code implies
	// it would add to *any* 'queries' list. If there are multiple, it adds to all
	// where it's missing. If there are none, it does nothing to 'queries'.
	// The provided snippet implies it adds to the *first* suitable one.
	// Let's refine to add to the first one encountered if not found.
	// Actually, the original code iterates and if not found in a specific list, it appends to THAT list.
	// So, if there are multiple 'queries' lists, it could be added to multiple.
	// This seems fine for typical sqlc.yml files.

	newConfigData, err := yaml.Marshal(sqlcConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal updated sqlc configuration: %w", err)
	}
	if err := os.WriteFile(sqlcConfigPath, newConfigData, 0644); err != nil {
		return fmt.Errorf("failed to update sqlc configuration file %s: %w", sqlcConfigPath, err)
	}

	fmt.Printf("Updated sqlc configuration at %s with new query file: %s\n", sqlcConfigPath, relativeQueryPath)
	return nil
}


// GenerateSQL is the original function, now acting as a wrapper.
// It will be removed or updated once the refactoring of main.go is complete.
func GenerateSQL(infraFile string) error {
	aiClient, err := NewAIClient()
	if err != nil {
		return fmt.Errorf("failed to create AI client: %w", err)
	}
	sg := NewSQLGenerator(aiClient)
	return sg.Generate(infraFile)
}
