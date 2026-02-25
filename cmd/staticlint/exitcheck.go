package main

import (
	"go/ast"
	"golang.org/x/tools/go/analysis"
)

// Analyzer, который мы добавляем в multichecker
var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "Запрещает прямой вызов os.Exit в функции main пакета main.\n",
	Run:  runExitCheck,
}

// run — основная функция анализатора
func runExitCheck(pass *analysis.Pass) (interface{}, error) {
	// Проходим по всем файлам текущего пакета
	for _, file := range pass.Files {

		// Проверяем только package main
		if file.Name.Name != "main" {
			continue
		}

		// Обходим всё дерево AST файла
		ast.Inspect(file, func(n ast.Node) bool {
			// Ищем объявления функций
			fn, ok := n.(*ast.FuncDecl)
			if !ok {
				return true // продолжаем обход
			}

			// Только func main() без приёмника (не метод)
			if fn.Name.Name != "main" || fn.Recv != nil {
				return true
			}

			// Теперь внутри тела main ищем os.Exit(...)
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Проверяем, что это именно os.Exit
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if sel.Sel.Name != "Exit" {
					return true
				}

				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "os" {
					return true
				}

				// Нашли запрещённый вызов!
				pass.Reportf(call.Pos(), "прямой вызов os.Exit() в main запрещён — "+
					"используйте return или log.Fatal / graceful shutdown")

				return true
			})

			return true
		})
	}

	return nil, nil
}
