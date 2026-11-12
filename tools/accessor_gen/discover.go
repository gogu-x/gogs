package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// discoverStructs 根据 go generate 提供的文件、行号定位指令所属结构体。
// 直接执行无参数命令时，扫描当前包所有带无参数生成指令的结构体。
func discoverStructs(directory, sourceFile string, sourceLine int) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		return nil, err
	}
	var names []string
	fset := token.NewFileSet()
	for _, file := range files {
		basename := filepath.Base(file)
		if sourceFile != "" && basename != sourceFile {
			continue
		}
		if strings.HasSuffix(basename, "_test.go") || strings.HasSuffix(basename, "_accessors.go") || basename == "deepcopy.go" {
			continue
		}
		source, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for _, decl := range source.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			groupMarked := hasAccessorDirective(fset, general.Doc, sourceLine)
			for _, spec := range general.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok || !(groupMarked || hasAccessorDirective(fset, typ.Doc, sourceLine)) {
					continue
				}
				if _, ok := typ.Type.(*ast.StructType); !ok {
					continue
				}
				if typ.Assign.IsValid() || typ.TypeParams != nil {
					return nil, fmt.Errorf("结构体 %s 暂不支持别名或泛型", typ.Name.Name)
				}
				names = append(names, typ.Name.Name)
			}
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("未找到目标结构体，请将 //go:generate accessor-gen 放在结构体声明上方，或显式传入结构体名")
	}
	return names, nil
}

// hasAccessorDirective 只匹配无参数指令；go generate 调用时还必须匹配当前行号。
func hasAccessorDirective(fset *token.FileSet, comments *ast.CommentGroup, line int) bool {
	if comments == nil {
		return false
	}
	for _, comment := range comments.List {
		if line > 0 && fset.Position(comment.Pos()).Line != line {
			continue
		}
		if strings.TrimSpace(comment.Text) == "//go:generate accessor-gen" {
			return true
		}
	}
	return false
}
