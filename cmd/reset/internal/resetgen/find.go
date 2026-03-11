package resetgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// FindResetableStructs ищет в пакете структуры, помеченные маркером //generate:reset.
//
// Функция обходит AST всех файлов пакета, ищет объявления типов (type),
// проверяет наличие маркера в комментариях и извлекает информацию о структуре
// и её полях для последующей генерации.
//
// Параметры:
//   - pkg: AST пакета, полученный от parser.ParseDir
//   - fset: файловый набор для определения позиций и преобразования типов
//   - pkgPath: полный путь к пакету на диске (для генерации файла)
//
// Возвращает: слайс ResetTarget с информацией о найденных структурах.
//
// Алгоритм работы:
// 1. Для каждого файла в пакете обходит AST в поисках *ast.GenDecl с токеном TYPE
// 2. Проверяет комментарии к объявлению на наличие строки "generate:reset"
// 3. Для каждого найденного TypeSpec проверяет, что это структура
// 4. Извлекает поля структуры через extractFields
// 5. Сохраняет результат в ResetTarget
func FindResetableStructs(pkg *ast.Package, fset *token.FileSet, pkgPath string) []ResetTarget {
	var targets []ResetTarget

	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			gen, ok := n.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				return true
			}

			// Проверяем наличие маркера в комментариях
			hasMarker := false
			if gen.Doc != nil {
				for _, c := range gen.Doc.List {
					line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
					if line == "generate:reset" {
						hasMarker = true
						break
					}
				}

				// Если маркера нет — пропускаем
				if !hasMarker {
					return true
				}

				// Обрабатываем все спецификации типов в этом объявлении
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					structType, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}

					// Создаём цель для генерации
					target := ResetTarget{
						PkgPath: pkgPath,
						PkgName: pkg.Name,
						Name:    ts.Name.Name,
						Fields:  extractFields(structType, fset),
					}
					targets = append(targets, target)
				}
				return true
			}
			return true
		})
	}
	return targets
}

// extractFields извлекает информацию о полях структуры из AST.
//
// Параметры:
//   - st: AST-узел структуры
//   - fset: файловый набор для преобразования типов
//
// Возвращает: слайс StructField с информацией о каждом поле.
//
// Особенности:
//   - Пропускает анонимные поля (без имени)
//   - Для каждого поля определяет имя, строковое представление типа и классификацию
//   - Выводит отладочную информацию (можно убрать или сделать условной)
func extractFields(st *ast.StructType, fset *token.FileSet) []StructField {
	var fields []StructField

	for _, field := range st.Fields.List {

		// Пропускаем поля без имени (встроенные структуры)
		if len(field.Names) == 0 {
			continue
		}

		name := field.Names[0].Name
		typeStr := exprToString(field.Type, fset)
		kind := classifyField(field.Type)

		// Отладочный вывод
		fmt.Printf("Поле %s → TypeStr = %q, Kind = %v\n", name, typeStr, kind)

		fields = append(fields, StructField{
			Name: name,
			Type: typeStr,
			Kind: kind,
		})
	}
	return fields
}
