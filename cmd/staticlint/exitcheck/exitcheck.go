package exitcheck

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer запрещает прямой вызов os.Exit в функции main пакета main.
//
// Причина: os.Exit немедленно завершает процесс, игнорируя все отложенные вызовы (defer),
// что может привести к утечкам ресурсов, незакрытым соединениям, несохранённым метрикам и т.д.
//
// Рекомендация: возвращать ошибку из main или использовать log.Fatal / graceful shutdown.
//
// Примеры:
//
//	// Правильно:
//	func main() {
//	    if err := run(); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
//	// Неправильно:
//	func main() {
//	    if err := run(); err != nil {
//	        os.Exit(1) // анализатор выдаст ошибку
//	    }
//	}
//
// Где используется: только в пакете main, в функции main.
var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "Запрещает прямой вызов os.Exit в функции main пакета main.\n",
	Run:  runExitCheck,
}

// runExitCheck — основная функция анализатора
//
// Алгоритм работы:
// 1. Проверяет, что анализируемый пакет называется "main"
// 2. Обходит AST в поисках функции main
// 3. Внутри тела main ищет вызовы os.Exit()
// 4. При нахождении сообщает об ошибке с указанием позиции в коде
//
// Возвращает: (nil, nil) — стандартный интерфейс анализатора.
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

				// Нашли запрещённый вызов
				pass.Reportf(call.Pos(), "прямой вызов os.Exit() в main запрещён — "+
					"используйте return или log.Fatal / graceful shutdown")

				return true
			})

			return true
		})
	}

	return nil, nil
}
