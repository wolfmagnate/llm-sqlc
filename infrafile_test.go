package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFirstInterface(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.go")
	source := `package sample

type MyInterface interface {
	Foo()
	Bar()
}

type MyInterfaceImpl struct {}

var _ MyInterface = MyInterfaceImpl{}

type OtherInterface interface {
	Baz()
}`
	if err := os.WriteFile(filePath, []byte(source), 0644); err != nil {
		t.Fatalf("failed to write temporary file: %v", err)
	}

	ifaceSrc, methods, implStructSrc, varCheckSrc, err := ExtractFirstInterface(filePath)
	if err != nil {
		t.Fatalf("ExtractFirstInterface() error: %v", err)
	}

	if !strings.Contains(ifaceSrc, "type MyInterface interface") {
		t.Errorf("expected interface declaration to contain 'type MyInterface interface', got: %q", ifaceSrc)
	}

	expectedMethods := []string{"Foo", "Bar"}
	if len(methods) != len(expectedMethods) {
		t.Fatalf("expected %d methods, got %d", len(expectedMethods), len(methods))
	}
	for i, m := range expectedMethods {
		if methods[i] != m {
			t.Errorf("expected method %q, got %q", m, methods[i])
		}
	}

	if !strings.Contains(implStructSrc, "MyInterfaceImpl") || !strings.Contains(implStructSrc, "struct") {
		t.Errorf("expected struct declaration for MyInterfaceImpl, got: %q", implStructSrc)
	}

	if !strings.Contains(varCheckSrc, "var _ MyInterface = MyInterfaceImpl{") {
		t.Errorf("expected var assignment for MyInterface, got: %q", varCheckSrc)
	}
}
