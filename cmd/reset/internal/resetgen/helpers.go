package resetgen

import (
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// exprToString преобразует ast.Expr в строковое представление типа.
//
// Использует go/printer для точного воспроизведения синтаксиса Go.
// Это необходимо для генерации корректного кода с правильными именами типов.
//
// Параметры:
//   - expr: AST-узел, представляющий выражение типа
//   - fset: файловый набор для определения позиций
//
// Возвращает: строковое представление типа (например, "[]string" или "map[string]int")
func exprToString(expr ast.Expr, fset *token.FileSet) string {
	var sb strings.Builder
	_ = printer.Fprint(&sb, fset, expr)
	return sb.String()
}

// classifyField определяет категорию типа поля для выбора стратегии сброса.
//
// Анализирует AST-узел и классифицирует тип по предопределённым категориям:
//   - Примитивные типы (int, string, bool и т.д.)
//   - Указатели
//   - Слайсы
//   - Мапы
//   - Структуры (встроенные и именованные)
//   - Специальные типы (time.Time обрабатывается как примитив)
//
// Параметры:
//   - expr: AST-узел с типом поля
//
// Возвращает: классификацию поля (FieldKind)
//
// Особенности:
//   - time.Time считается примитивом, так как сбрасывается через zero value
//   - Именованные типы-структуры помечаются как KindNamedStruct
//   - Безымянные структуры на месте — как KindEmbeddedStruct
func classifyField(expr ast.Expr) FieldKind {
	switch t := expr.(type) {
	case *ast.Ident:
		// Идентификатор может быть примитивом или именованным типом
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
		// Указатель на любой тип
		return KindPointer

	case *ast.ArrayType:
		// Слайс (если Len == nil) или массив фиксированной длины
		if t.Len == nil {
			return KindSlice
		}
		return KindPrimitive

	case *ast.MapType:
		// Мапа
		return KindMap

	case *ast.SelectorExpr:
		// Селектор (например, time.Time)
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "time" {
			if t.Sel.Name == "Time" {
				return KindPrimitive
			}
		}
		return KindNamedStruct

	case *ast.StructType:
		// Встроенная структура (anonymous field)
		return KindEmbeddedStruct

	default:
		// Неизвестный тип
		return KindUnknown
	}
}
