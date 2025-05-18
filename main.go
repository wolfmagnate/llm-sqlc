package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <path-to-infra-go-file>")
		os.Exit(1)
	}

	infraFile := os.Args[1]
	if err := GenerateInfra(infraFile); err != nil {
		log.Fatalf("error processing infra file: %v", err)
	}
}

func GenerateInfra(infraFile string) error {
	// 1. SQLコード生成
	if err := GenerateSQL(infraFile); err != nil {
		return fmt.Errorf("failed to generate SQL: %w", err)
	}

	// 2. プロジェクトルート(カレントディレクトリ)から "task generate-db" コマンドを実行
	cmd := exec.Command("task", "generate-db")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute task generate-db: %w", err)
	}

	// 3. プログラムコード生成
	if err := GenerateProgram(infraFile); err != nil {
		return fmt.Errorf("failed to generate program: %w", err)
	}

	return nil
}
