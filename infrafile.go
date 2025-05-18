package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
)

func ExtractFirstInterface(filePath string) (ifaceSrc string, methods []string, implStructSrc string, varCheckSrc string, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", nil, "", "", err
	}

	var interfaceName string
	foundInterface := false
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			interfaceName = ts.Name.Name

			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, genDecl); err != nil {
				return "", nil, "", "", err
			}
			ifaceSrc = buf.String()

			if it.Methods != nil {
				for _, field := range it.Methods.List {
					for _, name := range field.Names {
						methods = append(methods, name.Name)
					}
				}
			}
			foundInterface = true
			break
		}
		if foundInterface {
			break
		}
	}
	if !foundInterface {
		return "", nil, "", "", fmt.Errorf("no interface found in file %q", filePath)
	}

	targetStructName := interfaceName + "Impl"
	foundStruct := false
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != targetStructName {
				continue
			}
			if _, ok := ts.Type.(*ast.StructType); !ok {
				continue
			}
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, genDecl); err != nil {
				return "", nil, "", "", err
			}
			implStructSrc = buf.String()
			foundStruct = true
			break
		}
		if foundStruct {
			break
		}
	}
	if !foundStruct {
		return "", nil, "", "", fmt.Errorf("struct %q not found", targetStructName)
	}

	foundVar := false
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name != "_" {
					continue
				}
				idType, ok := vs.Type.(*ast.Ident)
				if !ok || idType.Name != interfaceName {
					continue
				}
				if len(vs.Values) == 0 {
					continue
				}
				cl, ok := vs.Values[0].(*ast.CompositeLit)
				if !ok {
					continue
				}
				idComposite, ok := cl.Type.(*ast.Ident)
				if !ok || idComposite.Name != targetStructName {
					continue
				}
				var buf bytes.Buffer
				if err := printer.Fprint(&buf, fset, genDecl); err != nil {
					return "", nil, "", "", err
				}
				varCheckSrc = buf.String()
				foundVar = true
				break
			}
			if foundVar {
				break
			}
		}
		if foundVar {
			break
		}
	}
	if !foundVar {
		return "", nil, "", "", fmt.Errorf("var _ %s = %s{} not found", interfaceName, targetStructName)
	}

	return ifaceSrc, methods, implStructSrc, varCheckSrc, nil
}
