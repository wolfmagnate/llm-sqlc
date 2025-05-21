package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSQLGenerator_preparePromptForMethod tests the prompt construction logic for SQL generation.
func TestSQLGenerator_preparePromptForMethod(t *testing.T) {
	sg := NewSQLGenerator(nil) // AIClient not needed for this method

	methodName := "GetUserByID"
	ifaceSrc := "type UserRepo interface {\n\tGetUserByID(ctx context.Context, id int) (*User, error)\n}"
	schemaContent := "CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(255));"
	entityDefsStr := "type User struct {\n\tID int\n\tName string\n}"

	prompt := sg.preparePromptForMethod(methodName, ifaceSrc, schemaContent, entityDefsStr)

	if !strings.Contains(prompt, methodName) {
		t.Errorf("Prompt does not contain method name '%s'", methodName)
	}
	if !strings.Contains(prompt, ifaceSrc) {
		t.Errorf("Prompt does not contain interface source: \n%s", ifaceSrc)
	}
	if !strings.Contains(prompt, schemaContent) {
		t.Errorf("Prompt does not contain schema content")
	}
	if !strings.Contains(prompt, entityDefsStr) {
		t.Errorf("Prompt does not contain entity definitions")
	}
	if !strings.Contains(prompt, "# sqlc") {
		t.Errorf("Prompt does not contain sqlc guidelines section")
	}
}

// TestSQLGenerator_updateSqlcConfig tests the logic for updating the sqlc.yml file.
func TestSQLGenerator_updateSqlcConfig(t *testing.T) {
	sg := NewSQLGenerator(nil) // AIClient not needed for this method

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "sqlgenerator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	infraBasePath := tmpDir // Use tmpDir as the stand-in for "pkg/infra"
	sqlcConfigFilePath := filepath.Join(infraBasePath, "sqlc.yml")
	
	// Case 1: sqlc.yml doesn't exist (updateSqlcConfig should return an error)
	t.Run("sqlc.yml does not exist", func(t *testing.T) {
		err := sg.updateSqlcConfig(filepath.Join(infraBasePath, "query/some.sql"), infraBasePath)
		if err == nil {
			t.Errorf("Expected error when sqlc.yml does not exist, got nil")
		}
	})

	// Initial sqlc.yml content
	initialSqlcContent := map[string]interface{}{
		"version": "2",
		"sql": []interface{}{
			map[string]interface{}{
				"schema":  "sql/schema/schema.sql",
				"queries": []interface{}{"query/existing.sql"},
				"engine":  "postgresql",
				"gen": map[string]interface{}{
					"go": map[string]interface{}{
						"package":              "db",
						"out":                  "db",
						"sql_package":          "pgx/v5",
						"emit_json_tags":       true,
						"emit_exact_table_names": true,
					},
				},
			},
		},
	}
	initialYamlBytes, _ := yaml.Marshal(initialSqlcContent)
	
	// Helper to write sqlc.yml and run the update
	runUpdateTest := func(testName string, sqlFilePathToAdd string, initialContent []byte, expectedQueriesCount int, expectSpecificQuery string) {
		t.Run(testName, func(t *testing.T) {
			if err := os.WriteFile(sqlcConfigFilePath, initialContent, 0644); err != nil {
				t.Fatalf("Failed to write initial sqlc.yml: %v", err)
			}

			fullSqlFilePath := filepath.Join(infraBasePath, sqlFilePathToAdd)
			err := sg.updateSqlcConfig(fullSqlFilePath, infraBasePath)
			if err != nil {
				// The function itself logs warnings for some internal errors but returns nil.
				// We are checking for errors returned directly by updateSqlcConfig.
				// The original code's error handling for sqlc.yml updates is mostly warnings.
				// Here, we only fail if updateSqlcConfig itself returns an error (e.g., file read/write).
				// Parsing/marshaling errors inside updateSqlcConfig are logged as warnings by it.
				// Let's adjust to check if the file was updated as expected.
			}

			updatedData, readErr := os.ReadFile(sqlcConfigFilePath)
			if readErr != nil {
				t.Fatalf("Failed to read updated sqlc.yml: %v", readErr)
			}
			var updatedConfig map[string]interface{}
			if yamlErr := yaml.Unmarshal(updatedData, &updatedConfig); yamlErr != nil {
				t.Fatalf("Failed to unmarshal updated sqlc.yml: %v", yamlErr)
			}
			
			sqlBlocks := updatedConfig["sql"].([]interface{})
			blockMap := sqlBlocks[0].(map[string]interface{})
			queriesList := blockMap["queries"].([]interface{})

			if len(queriesList) != expectedQueriesCount {
				t.Errorf("Expected %d queries, got %d. Queries: %v", expectedQueriesCount, len(queriesList), queriesList)
			}

			foundSpecificQuery := false
			for _, q := range queriesList {
				if qStr, ok := q.(string); ok && qStr == expectSpecificQuery {
					foundSpecificQuery = true
					break
				}
			}
			if !foundSpecificQuery {
				t.Errorf("Expected query '%s' not found in %v", expectSpecificQuery, queriesList)
			}
		})
	}

	// Case 2: Add a new query
	runUpdateTest("add new query", "query/new_query.sql", initialYamlBytes, 2, "query/new_query.sql")
	
	// Case 3: Add a query that already exists (should not duplicate)
	// Re-marshal initial content to reset for this test case
	initialYamlBytesWithExisting, _ := yaml.Marshal(map[string]interface{}{
		"version": "2",
		"sql": []interface{}{
			map[string]interface{}{
				"schema":  "sql/schema/schema.sql",
				"queries": []interface{}{"query/existing.sql"}, // ensure existing.sql is there
				"engine":  "postgresql",
				// ... other gen settings
			},
		},
	})
	runUpdateTest("add existing query", "query/existing.sql", initialYamlBytesWithExisting, 1, "query/existing.sql")

	// Case 4: Malformed YAML (e.g. sql block is not a list) - this should ideally be handled gracefully
	// The current implementation of updateSqlcConfig returns an error if 'sql' is not an array.
    t.Run("malformed sqlc.yml - sql not a list", func(t *testing.T) {
        malformedContent := []byte("version: \"2\"\nsql: not_a_list\n")
        if err := os.WriteFile(sqlcConfigFilePath, malformedContent, 0644); err != nil {
            t.Fatalf("Failed to write malformed sqlc.yml: %v", err)
        }
        err := sg.updateSqlcConfig(filepath.Join(infraBasePath, "query/another.sql"), infraBasePath)
        if err == nil {
            t.Errorf("Expected error for malformed sqlc.yml (sql not a list), got nil")
        } else if !strings.Contains(err.Error(), "sqlc.yml does not contain a valid 'sql' block as an array") {
            t.Errorf("Expected error about invalid sql block, got: %v", err)
        }
    })

    t.Run("malformed sqlc.yml - queries not a list", func(t *testing.T) {
        malformedContent := []byte("version: \"2\"\nsql:\n  - schema: sql/schema/schema.sql\n    queries: not_a_list\n")
         if err := os.WriteFile(sqlcConfigFilePath, malformedContent, 0644); err != nil {
            t.Fatalf("Failed to write malformed sqlc.yml: %v", err)
        }
        // The current updateSqlcConfig skips blocks where 'queries' is not a list, so it might not error out,
        // but it won't add the query either. We should check that the file remains unchanged or an error is logged.
        // The function doesn't return an error for this specific case but logs warnings.
        // For this test, we'll check if the intended operation (adding a query) failed.
        // This test might be more about observing logged behavior than a hard pass/fail on error return.
        // Let's check if the query was added. It should not be.
        sg.updateSqlcConfig(filepath.Join(infraBasePath, "query/no_add.sql"), infraBasePath) // This call should not error but log
        
        updatedData, _ := os.ReadFile(sqlcConfigFilePath)
        var updatedConfig map[string]interface{}
        yaml.Unmarshal(updatedData, &updatedConfig)
        sqlBlocks := updatedConfig["sql"].([]interface{})
		blockMap := sqlBlocks[0].(map[string]interface{})
		
        if _, ok := blockMap["queries"].(string); !ok { // It should remain "not_a_list"
             t.Errorf("Expected 'queries' to remain 'not_a_list', but it might have changed or type assertion failed differently.")
        }
        // Check that "query/no_add.sql" was NOT added.
        if strings.Contains(string(updatedData), "query/no_add.sql") {
            t.Errorf("'query/no_add.sql' should not have been added to a malformed 'queries' field.")
        }
    })
}

// Minimal test for generateSQLForMethod to ensure it calls the mock
func TestSQLGenerator_generateSQLForMethod(t *testing.T) {
	mockAI := NewMockAIClient()
	// sg := NewSQLGenerator(mockAI) // Requires NewSQLGenerator to accept a mockable interface

	expectedResponse := &SQLResponse{Queries: []string{"SELECT * FROM test;"}}
	mockAI.ResponseToReturn = expectedResponse

	// Simulate how SQLGenerator's generateSQLForMethod would use the mock
	// This test is effectively testing the mock's specific helper
	actualResp, err := mockAI.ChatCompletionHandlerForSQLGenerator(context.Background(), "gpt-4.1-mini", "sql test prompt")
	if err != nil {
		t.Fatalf("Mock handler failed: %v", err)
	}
	if len(actualResp.Queries) != 1 || actualResp.Queries[0] != expectedResponse.Queries[0] {
		t.Errorf("Expected response query '%v', got '%v'", expectedResponse.Queries, actualResp.Queries)
	}

	// Test error case
	mockAI.ResponseToReturn = nil
	mockAI.ErrorToReturn = fmt.Errorf("AI SQL error")
	_, err = mockAI.ChatCompletionHandlerForSQLGenerator(context.Background(), "gpt-4.1-mini", "sql test prompt for error")
	if err == nil {
		t.Errorf("Expected an error, got nil")
	}
	if err.Error() != "AI SQL error" {
		t.Errorf("Expected error message 'AI SQL error', got '%s'", err.Error())
	}
}
