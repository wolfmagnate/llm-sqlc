package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EntityDefinition は、対象ファイルのパスと、抽出したコード（package/importを除く）を保持します。
type EntityDefinition struct {
	FileName string // ファイルパス
	Code     string // New関数まで（およびそれ以前）の宣言群をプリントした結果
}

// ExtractEntityDefinitions は、rootDir 以下（再帰的）にある Go ファイルのうち、
// ・ファイル名から導出したエンティティ名に対応する type 宣言が存在し、
// ・かつ func NewXxx(...) 宣言が存在するものについて、
// New関数の終了位置までの宣言（import 宣言は除く）を printer で整形し、結果を構造体のスライスとして返します。
func ExtractEntityDefinitions(rootDir string) ([]EntityDefinition, error) {
	var results []EntityDefinition
	fset := token.NewFileSet()

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// ディレクトリはスキップ
		if d.IsDir() {
			return nil
		}
		// .go ファイルのみ対象（\_test.go は除外）
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		// ソースを読み込んで AST へパース（コメントも取得）
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileAst, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}

		// ファイル名（拡張子除く）からエンティティ名を算出（例："user" → "User"）
		baseName := strings.TrimSuffix(d.Name(), ".go")
		if baseName == "" {
			return nil
		}
		entityName := strings.ToUpper(baseName[:1]) + baseName[1:]

		// AST 上の宣言群から、エンティティの type 宣言と、func NewXxx 宣言を探す
		var foundType bool
		var newFuncDecl *ast.FuncDecl
		for _, decl := range fileAst.Decls {
			// type 宣言のチェック
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if ts.Name.Name == entityName {
							foundType = true
						}
					}
				}
			}
			// func NewXxx 宣言のチェック
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if funcDecl.Name.Name == "New"+entityName {
					newFuncDecl = funcDecl
				}
			}
		}
		// 両方見つからなければスキップ
		if !foundType || newFuncDecl == nil {
			return nil
		}

		// New 関数の終了位置（オフセット）を取得
		newFuncEndOffset := fset.Position(newFuncDecl.End()).Offset

		// ファイル内の宣言のうち、New関数の終了位置より前（または同位置）のものを抽出
		// ただし、import 宣言（GenDecl で Tok==IMPORT）は除外する
		var filteredDecls []ast.Decl
		for _, decl := range fileAst.Decls {
			if fset.Position(decl.End()).Offset <= newFuncEndOffset {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
					continue
				}
				filteredDecls = append(filteredDecls, decl)
			}
		}

		// printer.Fprint を用いて、フィルタ済みの宣言群をコードとして再整形する
		var buf bytes.Buffer
		for _, decl := range filteredDecls {
			if err := printer.Fprint(&buf, fset, decl); err != nil {
				return err
			}
			buf.WriteString("\n")
		}

		results = append(results, EntityDefinition{
			FileName: path,
			Code:     buf.String(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
