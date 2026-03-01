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
		return KindPrimitive

	case *ast.StarExpr:
		return KindPointer

	case *ast.ArrayType:
		if t.Len == nil {
			return KindSlice
		}
		return KindPrimitive

	case *ast.MapType:
		return KindMap

	case *ast.StructType:
		return KindStruct

	default:
		return KindUnknown
	}
}
