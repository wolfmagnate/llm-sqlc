package main

import (
	"context"
	"fmt"
	"strings" // Moved import to the top
)

// MockAIClient is a mock implementation of AIClient for testing.
type MockAIClient struct {
	ChatCompletionHandlerFunc func(ctx context.Context, model string, prompt string) (interface{}, error)
	ExpectedModel             string
	ExpectedPromptContains    string // Substring to check in the prompt
	ResponseToReturn          interface{}
	ErrorToReturn             error
}

// NewMockAIClient creates a new MockAIClient.
func NewMockAIClient() *MockAIClient {
	return &MockAIClient{}
}

// ChatCompletionHandler mocks the AIClient's ChatCompletionHandler method.
// It allows tests to set expected responses or custom handler functions.
// Note: This simplified mock uses interface{} for ResponseToReturn.
// Tests will need to ensure ResponseToReturn is of the correct specific type
// (e.g., *GenerationResponse or *SQLResponse) expected by the calling code.
// A more advanced mock could use generics if the Go version supports it for methods,
// or provide type-specific mock methods.
func (m *MockAIClient) ChatCompletionHandler(ctx context.Context, model string, prompt string) (interface{}, error) {
	if m.ChatCompletionHandlerFunc != nil {
		return m.ChatCompletionHandlerFunc(ctx, model, prompt)
	}
	if m.ExpectedModel != "" && m.ExpectedModel != model {
		return nil, fmt.Errorf("mock AIClient: expected model %s, got %s", m.ExpectedModel, model)
	}
	if m.ExpectedPromptContains != "" && !strings.Contains(prompt, m.ExpectedPromptContains) {
		return nil, fmt.Errorf("mock AIClient: prompt does not contain expected substring '%s'", m.ExpectedPromptContains)
	}
	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	return m.ResponseToReturn, nil
}

// ChatCompletionHandlerForProgramGenerator is a helper for ProgramGenerator tests.
// It casts the generic ResponseToReturn to *GenerationResponse.
func (m *MockAIClient) ChatCompletionHandlerForProgramGenerator(ctx context.Context, model string, prompt string) (*GenerationResponse, error) {
	if m.ChatCompletionHandlerFunc != nil {
		// If a custom func is provided, it must handle the type assertion itself or be specific.
		// This example assumes the custom func, if any, returns the correct type or this cast will fail.
		rawResp, err := m.ChatCompletionHandlerFunc(ctx, model, prompt)
		if err != nil {
			return nil, err
		}
		if typedResp, ok := rawResp.(*GenerationResponse); ok {
			return typedResp, nil
		}
		return nil, fmt.Errorf("mock AIClient: ChatCompletionHandlerFunc returned unexpected type: %T for GenerationResponse", rawResp)
	}

	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	if typedResp, ok := m.ResponseToReturn.(*GenerationResponse); ok {
		return typedResp, nil
	}
    if m.ResponseToReturn == nil { // Handle cases where nil response is expected with nil error
        return nil, nil
    }
	return nil, fmt.Errorf("mock AIClient: ResponseToReturn is of unexpected type: %T for GenerationResponse", m.ResponseToReturn)
}

// ChatCompletionHandlerForSQLGenerator is a helper for SQLGenerator tests.
// It casts the generic ResponseToReturn to *SQLResponse.
func (m *MockAIClient) ChatCompletionHandlerForSQLGenerator(ctx context.Context, model string, prompt string) (*SQLResponse, error) {
	if m.ChatCompletionHandlerFunc != nil {
		rawResp, err := m.ChatCompletionHandlerFunc(ctx, model, prompt)
		if err != nil {
			return nil, err
		}
		if typedResp, ok := rawResp.(*SQLResponse); ok {
			return typedResp, nil
		}
		return nil, fmt.Errorf("mock AIClient: ChatCompletionHandlerFunc returned unexpected type: %T for SQLResponse", rawResp)
	}

	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	if typedResp, ok := m.ResponseToReturn.(*SQLResponse); ok {
		return typedResp, nil
	}
    if m.ResponseToReturn == nil { // Handle cases where nil response is expected with nil error
        return nil, nil
    }
	return nil, fmt.Errorf("mock AIClient: ResponseToReturn is of unexpected type: %T for SQLResponse", m.ResponseToReturn)
}
