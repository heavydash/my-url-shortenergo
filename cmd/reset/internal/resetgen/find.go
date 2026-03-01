package resetgen

import (
	"go/ast"
	"go/token"
	"strings"
)

func FindResetableStructs(pkg *ast.Package, fset *token.FileSet) []ResetTarget {
	var targets []ResetTarget

	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			gen, ok := n.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				return true
			}

			hasMarker := false
			for _, c := range gen.Doc.List {
				if strings.TrimSpace(c.Text) == "// generate:reset" {
					hasMarker = true
					break
				}
			}
			if !hasMarker {
				return true
			}

			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				structType, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				target := ResetTarget{
					Name:   ts.Name.Name,
					Fields: extractFields(structType, fset),
				}
				targets = append(targets, target)
			}
			return true
		})
	}
	return targets
}

func extractFields(st *ast.StructType, fset *token.FileSet) []StructField {
	var fields []StructField

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		name := field.Names[0].Name
		typeStr := exprToString(field.Type, fset)
		kind := classifyField(field.Type)

		fields = append(fields, StructField{
			Name: name,
			Type: typeStr,
			Kind: kind,
		})
	}
	return fields
}
