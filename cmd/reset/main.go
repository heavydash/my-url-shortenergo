package main

import (
	"flag"
	"fmt"
	"github.com/heavydash/my-url-shortenergo/cmd/reset/internal/resetgen"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := flag.String("dir", ".", "The root directory of the project")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	fmt.Printf("finder of structure with // generate:reset in %s\n\n", *root)

	var walkErr error
	walkErr = filepath.WalkDir(*root, func(path string, d os.DirEntry, err error) error {
		if walkErr != nil {
			return walkErr
		}
		// Обрабатываем только директории
		if !d.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if base == ".git" || base == "vendor" || strings.HasPrefix(base, ".") {
			return filepath.SkipDir
		}

		matches, _ := filepath.Glob(filepath.Join(path, "*.go"))
		if len(matches) == 0 {
			return nil
		}

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Printf("parse error %s: %v\n", path, err)
			return nil
		}

		for _, pkg := range pkgs {
			targets := resetgen.FindResetableStructs(pkg, fset)
			// Если ничего не нашли, молчим
			if len(targets) == 0 {
				continue
			}

			rel, _ := filepath.Rel(*root, path)
			pkgPath := rel
			if pkgPath == "." {
				pkgPath = "(root)"
			}

			// В обычном режиме выводим пакеты со структурами
			fmt.Printf("Pckg %s (%s)\n", pkg.Name, rel)

			for _, t := range targets {
				fmt.Printf("\t%s\n", t.Name)

				if *verbose {
					for _, f := range t.Fields {
						fmt.Printf(" %-20s %-30s %v\n", f.Name, f.Type, f.Kind)
					}
					fmt.Println()
				}
			}
		}

		return nil
	})

	if walkErr != nil {
		fmt.Fprintln(os.Stderr, "WalkDir error: ", walkErr)
		os.Exit(1)
	}
	fmt.Println("\nСканирование завершено.")
}
