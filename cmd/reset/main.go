package main

import (
	"flag"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/cmd/reset/internal/resetgen"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", ".", "dir for scanning")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	fmt.Printf(" Searching of structs с // generate:reset в %s\n\n", *dir)

	fset := token.NewFileSet()

	pkgTargets := make(map[string][]resetgen.ResetTarget)

	err := filepath.WalkDir(*dir, func(path string, d os.DirEntry, err error) error {
		// Если ошибка доступа — печатаем, но продолжаем обход
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to access %s: %v\n", path, err)
			return nil // Если return err, весь обход стопнется
		}

		// Обработка директории
		if !d.IsDir() {
			return nil
		}

		// Пропускаем служебные папки
		base := filepath.Base(path)
		if base == ".git" || base == "vendor" || base == "node_modules" {
			return filepath.SkipDir
		}

		//  Отладка, показываем, куда зашли
		rel, _ := filepath.Rel(*dir, path)
		if rel == "." {
			rel = "(root)"
		}
		fmt.Printf("→ %s\n", rel)

		// Проверка go файлов
		matches, err := filepath.Glob(filepath.Join(path, "*.go"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "glob error %s: %v\n", path, err)
			return nil
		}
		if len(matches) == 0 {
			return nil
		}

		// Парсим пакет
		pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error %s: %v\n", path, err)
			return nil
		}

		// Поиск структур
		for pkgName, pkg := range pkgs {
			// Передаём путь к директории пакета
			targets := resetgen.FindResetableStructs(pkg, fset, path)

			if len(targets) == 0 {
				continue
			}

			fmt.Printf("  Package %s — find structs: %d\n", pkgName, len(targets))

			// Добавляем все найденные структуры в карту по пути пакета
			pkgTargets[path] = append(pkgTargets[path], targets...)

			// Вывод
			if *verbose {
				for _, t := range targets {
					fmt.Printf("    %s\n", t.Name)
					for _, f := range t.Fields {
						fmt.Printf("      %-15s %-25s %v\n", f.Name, f.Type, f.Kind)
					}
					fmt.Println()
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "WalkDir failed with err: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n Scanning has been finished")

	for pkgPath, targets := range pkgTargets {
		if len(targets) == 0 {
			continue
		}

		fmt.Printf("Gen of package %s (%d structs)\n", pkgPath, len(targets))

		err := resetgen.GenerateResetMethods(pkgPath, targets)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error of gen %s: %v\n", pkgPath, err)
		}
	}

	fmt.Println("\n Scanning has been finished")
}
