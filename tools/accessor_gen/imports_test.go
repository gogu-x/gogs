package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"
)

// TestImportedAccessorsCompile 验证外部类型的访问器能与源文件一起通过类型检查。
func TestImportedAccessorsCompile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "model.go")
	if err := os.WriteFile(source, []byte("package sample\nimport clock \"time\"\ntype Data struct { At clock.Time }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := generateAccessors(dir, []string{"Data"}); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, path := range []string{source, filepath.Join(dir, "data_accessors.go")} {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	config := types.Config{Importer: importer.Default()}
	if _, err := config.Check("sample", fset, files, nil); err != nil {
		t.Fatal(err)
	}
}
