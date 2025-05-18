package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateInfra(t *testing.T) {
	// カレントディレクトリを指定のパスに変更
	targetDir := `C:\Users\wolfm\Documents\就活\CyberAgent\ace-c-server`
	if err := os.Chdir(targetDir); err != nil {
		t.Fatalf("カレントディレクトリの変更に失敗しました: %v", err)
	}

	// テスト対象のファイルパス（相対パス）
	infraFile := filepath.Join("pkg", "infra", "user.go")

	// generateInfra を実行
	if err := GenerateSQL(infraFile); err != nil {
		t.Fatalf("generateInfra の実行に失敗しました: %v", err)
	}

	// 出力ファイルは、元のファイル名（拡張子除去）に .sql を付与して作成される
	expectedOutputFile := filepath.Join("pkg", "infra", "sql", "query", "user.sql")
	if info, err := os.Stat(expectedOutputFile); err != nil {
		t.Fatalf("期待される出力ファイル %s が存在しません: %v", expectedOutputFile, err)
	} else if info.IsDir() {
		t.Fatalf("期待される出力ファイル %s はディレクトリになっています", expectedOutputFile)
	}
}

func TestGenerateInfraProgram(t *testing.T) {
	// カレントディレクトリを指定のパスに変更
	targetDir := `C:\Users\wolfm\Documents\就活\CyberAgent\ace-c-server`
	if err := os.Chdir(targetDir); err != nil {
		t.Fatalf("カレントディレクトリの変更に失敗しました: %v", err)
	}

	// テスト対象のファイルパス（相対パス）
	infraFile := filepath.Join("pkg", "infra", "user.go")

	// generateInfra を実行
	if err := GenerateProgram(infraFile); err != nil {
		t.Fatalf("generateInfra の実行に失敗しました: %v", err)
	}

	// 出力ファイルは、元のファイル名（拡張子除去）に .sql を付与して作成される
	expectedOutputFile := filepath.Join("pkg", "infra", "sql", "query", "user.sql")
	if info, err := os.Stat(expectedOutputFile); err != nil {
		t.Fatalf("期待される出力ファイル %s が存在しません: %v", expectedOutputFile, err)
	} else if info.IsDir() {
		t.Fatalf("期待される出力ファイル %s はディレクトリになっています", expectedOutputFile)
	}
}
