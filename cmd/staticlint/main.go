// Пакет main реализует мультичекер — объединённый инструмент статического анализа.
//
// Мультичекер предназначен для автоматической проверки кода проекта сокращателя URL
// и включает в себя набор стандартных, публичных и кастомных анализаторов.
//
// Особенности:
//   - Проверка критических ошибок (SAxxxx из staticcheck)
//   - Проверка стиля оформления (ST1000)
//   - Проверка распространённых ошибок (errcheck, nilerr)
//   - Кастомная проверка на os.Exit в main
//
// Запуск:
//
//	go run ./cmd/staticlint ./...
//
// Для запуска с флагами (например, для сохранения отчёта в файл):
//
//	go run ./cmd/staticlint -json -output report.json ./...
package main

import (
	"github.com/gostaticanalysis/nilerr"
	"github.com/heavydash/my-url-shortenergo/cmd/staticlint/exitcheck"
	"github.com/kisielk/errcheck/errcheck"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"

	"strings"

	"honnef.co/go/tools/staticcheck"
)

// Multichecker — объединённый инструмент статического анализа для проекта сокращателя URL.
//
// Запуск:
//
//	go run ./cmd/staticlint ./...
//
// Подключённые анализаторы:
//   - Стандартные passes: assign, shadow, printf, nilness, lostcancel, copylock, loopclosure
//   - Все SAxxxx из staticcheck (критические баги)
//   - ST1000 — проверка стиля и комментариев к пакету
//   - Публичные: errcheck (непроверенные ошибки), nilerr (nil в ошибках)
//   - Кастомный: exitcheck — запрет прямого вызова os.Exit в функции main пакета main
//
// Структура анализаторов:
//   - Стандартные анализаторы (golang.org/x/tools/go/analysis/passes):
//     Базовые проверки, включённые в стандартный набор go vet.
//   - Staticcheck SA (honnef.co/go/tools/staticcheck):
//     Критические ошибки и проблемы надёжности кода.
//   - Стилевой анализатор ST1000:
//     Проверка наличия и качества комментариев к пакету.
//   - Публичные анализаторы:
//     errcheck — требует явной проверки возвращаемых ошибок
//     nilerr — находит ситуации, когда возвращается nil вместо ошибки
//   - Кастомный анализатор exitcheck:
func main() {
	// Инициализация слайса анализаторов
	analyzers := []*analysis.Analyzer{
		assign.Analyzer,      // бесполезные присваивания
		shadow.Analyzer,      // затенения переменных
		printf.Analyzer,      // ошибки fmt.Printf
		nilness.Analyzer,     // nil-difference
		lostcancel.Analyzer,  // забытый cancel контекста
		copylock.Analyzer,    // Копирование заблокированного mutex
		loopclosure.Analyzer, // замыкание в циклах
	}
	// SA анализаторы из staticcheck
	// Включают все проверки категории SA (static analysis),
	// которые выявляют критически важные ошибки:
	// - SA1000: неправильная работа с регулярными выражениями
	// - SA1004: неправильная работа с time.Sleep
	// - SA1012: неправильный контекст
	// - и другие SAxxxx проверки
	for _, sa := range staticcheck.Analyzers {
		if strings.HasPrefix(sa.Analyzer.Name, "SA") {
			analyzers = append(analyzers, sa.Analyzer)
		}
	}
	// Добавляем стилевой анализатор ST1000
	// Проверяет, что у пакета есть осмысленный комментарий.
	for _, sa := range staticcheck.Analyzers {
		if sa.Analyzer.Name == "ST1000" {
			analyzers = append(analyzers, sa.Analyzer)
			break
		}
	}
	// Кастомный анализатор exitcheck
	// Запрещает прямой вызов os.Exit в main, так как это может привести к
	// пропуску отложенных вызовов (defer) и утечкам ресурсов.
	analyzers = append(analyzers, exitcheck.Analyzer)

	// Публичные анализаторы
	// errcheck — проверяет, что все ошибки обработаны
	// nilerr — находит случаи, когда функция возвращает nil вместо ошибки
	analyzers = append(analyzers, errcheck.Analyzer)
	analyzers = append(analyzers, nilerr.Analyzer)

	// Запуск мультичекера
	// multichecker.Main запускает все анализаторы и выводит результаты
	// Поддерживает стандартные флаги: -json, -output и другие
	multichecker.Main(analyzers...)
}
