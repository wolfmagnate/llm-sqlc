package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractEntityDefinitions は、ExtractEntityDefinitions の動作を検証するユニットテストです。
func TestExtractEntityDefinitions(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "entity_test")
	if err != nil {
		t.Fatalf("一時ディレクトリの作成に失敗: %v", err)
	}
	// テスト終了後に一時ディレクトリを削除
	defer os.RemoveAll(tempDir)

	// ヘルパー関数: 指定された相対パスにファイルを作成する
	writeFile := func(relPath, content string) {
		fullPath := filepath.Join(tempDir, relPath)
		// 必要なディレクトリを作成
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("ディレクトリ %s の作成に失敗: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("ファイル %s の書き込みに失敗: %v", fullPath, err)
		}
	}

	// -------------------------------
	// テスト用ソースコードを各ファイルに作成
	// -------------------------------

	// 1. ルート: 有効なファイル "user.go"
	writeFile("user.go", `
package main
import "fmt"
type User struct {}
func NewUser() *User {
	return &User{}
}
func extra() {}
`)

	// 2. ルート: 無効なファイル "order.go"（ファイル名から Order と期待されるが、定義がない）
	writeFile("order.go", `
package main
type SomethingElse struct {}
func NewSomethingElse() *SomethingElse {
	return &SomethingElse{}
}
`)

	// 3. サブディレクトリ "entity": 有効なファイル "product.go"
	writeFile("entity/product.go", `
package entity
import "fmt"
type Product struct {}
func NewProduct() *Product {
	return &Product{}
}
`)

	// 4. サブディレクトリ "entity": テスト用ファイル "dummy_test.go"（除外される）
	writeFile("entity/dummy_test.go", `
package entity
import "testing"
func TestDummy(t *testing.T) {}
`)

	// 5. サブディレクトリ "other": 無効なファイル "customer.go"（NewCustomer がない）
	writeFile("other/customer.go", `
package other
type Customer struct {}
`)

	// 6. サブディレクトリ "other": 有効なファイル "invoice.go"
	writeFile("other/invoice.go", `
package other
type Invoice struct {}
func NewInvoice() *Invoice {
	return &Invoice{}
}
`)

	// 7. ネストしたサブディレクトリ "entity/sub": 有効なファイル "account.go"
	writeFile("entity/sub/account.go", `
package sub
type Account struct {}
func NewAccount() *Account {
	return &Account{}
}
`)

	// -------------------------------
	// ExtractEntityDefinitions の実行
	// -------------------------------
	defs, err := ExtractEntityDefinitions(tempDir)
	if err != nil {
		t.Fatalf("ExtractEntityDefinitions の実行でエラー: %v", err)
	}

	// 有効なファイルとして期待するパス
	expectedFiles := map[string]bool{
		filepath.Join(tempDir, "user.go"):               true,
		filepath.Join(tempDir, "entity/product.go"):     true,
		filepath.Join(tempDir, "other/invoice.go"):      true,
		filepath.Join(tempDir, "entity/sub/account.go"): true,
	}

	// 取得されたファイルの一覧を確認
	found := make(map[string]bool)
	for _, def := range defs {
		t.Logf("抽出されたファイル: %s", def.FileName)
		found[def.FileName] = true
	}

	// 期待する有効なファイルがすべて抽出されているか検証
	for file := range expectedFiles {
		if !found[file] {
			t.Errorf("期待するファイル %s が抽出されていません", file)
		}
	}

	// 期待しないファイルが抽出結果に含まれていないか検証
	unexpectedFiles := []string{
		filepath.Join(tempDir, "order.go"),
		filepath.Join(tempDir, "entity/dummy_test.go"),
		filepath.Join(tempDir, "other/customer.go"),
	}
	for _, file := range unexpectedFiles {
		if found[file] {
			t.Errorf("抽出結果に含まれてはならないファイル %s が見つかりました", file)
		}
	}

	// 例: user.go の抽出結果のコードに package/import 文が含まれていないことを確認
	for _, def := range defs {
		if filepath.Base(def.FileName) == "user.go" {
			if strings.Contains(def.Code, "package") || strings.Contains(def.Code, "import") {
				t.Errorf("ファイル %s の抽出コードに package/import 文が含まれています", def.FileName)
			}
		}
	}
}
