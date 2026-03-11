package resetgen

import (
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

func exprToString(expr ast.Expr, fset *token.FileSet) string {
	var sb strings.Builder
	_ = printer.Fprint(&sb, fset, expr)
	return sb.String()
}

func classifyField(expr ast.Expr) FieldKind {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "bool", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"float32", "float64", "complex64", "complex128",
			"byte", "rune", "string":
			return KindPrimitive
		default:
			return KindNamedStruct
		}
	case *ast.StarExpr:
		return KindPointer

	case *ast.ArrayType:
		if t.Len == nil {
			return KindSlice
		}
		return KindPrimitive

	case *ast.MapType:
		return KindMap

	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "time" {
			if t.Sel.Name == "Time" {
				return KindPrimitive
			}
		}
		return KindNamedStruct

	case *ast.StructType:
		return KindEmbeddedStruct

	default:
		return KindUnknown
	}
}
