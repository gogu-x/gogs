// Command deepcopy-gen generates DeepCopy methods for marked structs.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const generatedFileName = "deepcopy.go"

type packageInfo struct {
	dir        string
	importPath string
	name       string
	files      []*ast.File
	types      map[string]*structInfo
	marked     bool
}

type structInfo struct {
	name     string
	fields   []*ast.Field
	marked   bool
	excluded bool
}

type generator struct {
	root     string
	packages map[string]*packageInfo
	byImport map[string]*packageInfo
	current  *packageInfo
	imports  map[string]string
	sequence int
}

func main() {
	rootFlag := flag.String("dir", ".", "root directory to scan recursively")
	flag.Parse()

	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatal(err)
	}
	g := &generator{
		root:     root,
		packages: make(map[string]*packageInfo),
		byImport: make(map[string]*packageInfo),
	}
	if err := g.loadPackages(); err != nil {
		fatal(err)
	}
	if err := g.generate(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "deepcopy-gen:", err)
	os.Exit(1)
}

func (g *generator) loadPackages() error {
	modulePath, moduleRoot, err := findModulePath(g.root)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	err = filepath.WalkDir(g.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != g.root && (entry.Name() == ".git" || entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == generatedFileName {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		dir, relErr := filepath.Rel(moduleRoot, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		if relErr == nil && dir == "." {
			dir = ""
		}
		absoluteDir := filepath.Clean(filepath.Dir(path))
		pkg := g.packages[absoluteDir]
		if pkg == nil {
			importPath := modulePath
			if dir != "" {
				importPath += "/" + filepath.ToSlash(dir)
			}
			pkg = &packageInfo{
				dir: absoluteDir, importPath: importPath, name: file.Name.Name,
				types: make(map[string]*structInfo),
			}
			g.packages[absoluteDir] = pkg
			g.byImport[importPath] = pkg
		}
		if pkg.name != file.Name.Name {
			return fmt.Errorf("multiple package names in %s", absoluteDir)
		}
		pkg.files = append(pkg.files, file)
		for _, declaration := range file.Decls {
			genDecl, ok := declaration.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				pkg.types[typeSpec.Name.Name] = &structInfo{
					name: typeSpec.Name.Name, fields: structType.Fields.List,
					marked:   hasMarker(genDecl.Doc, "+deepcopy-gen=true") || hasMarker(typeSpec.Doc, "+deepcopy-gen=true"),
					excluded: hasMarker(genDecl.Doc, "+deepcopy-gen=false") || hasMarker(typeSpec.Doc, "+deepcopy-gen=false"),
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, pkg := range g.packages {
		for _, file := range pkg.files {
			pkg.marked = pkg.marked || hasMarker(file.Doc, "+deepcopy-gen=package")
		}
		if !pkg.marked {
			continue
		}
		for _, info := range pkg.types {
			if !info.excluded {
				info.marked = true
			}
		}
	}
	return nil
}

func findModulePath(start string) (string, string, error) {
	dir := start
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "module" {
					return fields[1], dir, nil
				}
			}
			return "", "", fmt.Errorf("module path not found in %s", filepath.Join(dir, "go.mod"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("could not find go.mod above %s", start)
		}
		dir = parent
	}
}

func hasMarker(group *ast.CommentGroup, marker string) bool {
	if group == nil {
		return false
	}
	for _, comment := range group.List {
		if strings.TrimSpace(strings.TrimPrefix(comment.Text, "//")) == marker {
			return true
		}
	}
	return false
}

func (g *generator) generate() error {
	packageDirs := make([]string, 0, len(g.packages))
	for dir := range g.packages {
		packageDirs = append(packageDirs, dir)
	}
	sort.Strings(packageDirs)
	for _, dir := range packageDirs {
		pkg := g.packages[dir]
		marked := false
		typeNames := make([]string, 0, len(pkg.types))
		for name := range pkg.types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		for _, name := range typeNames {
			info := pkg.types[name]
			marked = marked || info.marked
		}
		if !marked {
			continue
		}
		g.current = pkg
		g.imports = g.packageImports(pkg)
		g.sequence = 0
		contents, err := g.generatePackage()
		if err != nil {
			return fmt.Errorf("%s: %w", pkg.dir, err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), generatedFileName, contents, parser.AllErrors); err != nil {
			return fmt.Errorf("parse generated code for %s: %w\n%s", pkg.dir, err, contents)
		}
		outputPath := filepath.Join(pkg.dir, generatedFileName)
		if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
			return err
		}
		if output, err := exec.Command("gofmt", "-w", outputPath).CombinedOutput(); err != nil {
			return fmt.Errorf("gofmt %s: %w\n%s", outputPath, err, output)
		}
		fmt.Printf("generated %s\n", outputPath)
	}
	return nil
}

func (g *generator) packageImports(pkg *packageInfo) map[string]string {
	imports := make(map[string]string)
	for _, file := range pkg.files {
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			alias := path.Base(importPath)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			imports[alias] = importPath
		}
	}
	return imports
}

func (g *generator) generatePackage() ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintf(&output, "// Code generated by deepcopy-gen. DO NOT EDIT.\n\npackage %s\n\n", g.current.name)
	typeNames := make([]string, 0, len(g.current.types))
	for name := range g.current.types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		info := g.current.types[name]
		if !info.marked {
			continue
		}
		fmt.Fprintf(&output, "// DeepCopy 返回 %s 的独立副本。\n", info.name)
		fmt.Fprintf(&output, "func (in *%s) DeepCopy() *%s {\n", info.name, info.name)
		output.WriteString("\tif in == nil {\n\t\treturn nil\n\t}\n\tout := *in\n")
		for _, field := range info.fields {
			if len(field.Names) == 0 {
				return nil, fmt.Errorf("embedded fields are not supported in %s", info.name)
			}
			for _, name := range field.Names {
				if identifier, ok := field.Type.(*ast.Ident); ok && isPrimitive(identifier.Name) {
					continue
				}
				if err := g.emitClone(&output, "out."+name.Name, "in."+name.Name, field.Type, 1); err != nil {
					return nil, fmt.Errorf("%s.%s: %w", info.name, name.Name, err)
				}
			}
		}
		output.WriteString("\treturn &out\n}\n\n")
	}
	return output.Bytes(), nil
}

func (g *generator) emitClone(output *bytes.Buffer, destination, source string, expression ast.Expr, indent int) error {
	prefix := strings.Repeat("\t", indent)
	switch typed := expression.(type) {
	case *ast.StarExpr:
		if name, packageName, ok := namedType(typed.X); ok {
			if g.isMarked(packageName, name) {
				fmt.Fprintf(output, "%s%s = %s.DeepCopy()\n", prefix, destination, source)
				return nil
			}
			if g.isStruct(packageName, name) || packageName != "" || !isPrimitive(name) {
				return fmt.Errorf("pointer to unmarked struct %s requires +deepcopy-gen=true", name)
			}
		}
		value := g.next("value")
		fmt.Fprintf(output, "%sif %s != nil {\n%s\t%s := *%s\n%s\t%s = &%s\n%s}\n", prefix, source, prefix, value, source, prefix, destination, value, prefix)
	case *ast.MapType:
		keyName := g.next("key")
		valueName := g.next("value")
		clonedValue := g.next("cloned")
		valueType, err := g.nodeString(typed.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%sif %s != nil {\n%s\t%s = make(%s, len(%s))\n%s\tfor %s, %s := range %s {\n%s\t\tvar %s %s\n", prefix, source, prefix, destination, g.mustNodeString(expression), source, prefix, keyName, valueName, source, prefix, clonedValue, valueType)
		if err := g.emitClone(output, clonedValue, valueName, typed.Value, indent+2); err != nil {
			return err
		}
		fmt.Fprintf(output, "%s\t\t%s[%s] = %s\n%s\t}\n%s}\n", prefix, destination, keyName, clonedValue, prefix, prefix)
	case *ast.ArrayType:
		if typed.Len == nil {
			index := g.next("index")
			item := g.next("item")
			clonedValue := g.next("cloned")
			valueType, err := g.nodeString(typed.Elt)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "%sif %s != nil {\n%s\t%s = make(%s, len(%s))\n%s\tfor %s, %s := range %s {\n%s\t\tvar %s %s\n", prefix, source, prefix, destination, g.mustNodeString(expression), source, prefix, index, item, source, prefix, clonedValue, valueType)
			if err := g.emitClone(output, clonedValue, item, typed.Elt, indent+2); err != nil {
				return err
			}
			fmt.Fprintf(output, "%s\t\t%s[%s] = %s\n%s\t}\n%s}\n", prefix, destination, index, clonedValue, prefix, prefix)
			return nil
		}
		index := g.next("index")
		fmt.Fprintf(output, "%sfor %s := range %s {\n", prefix, index, source)
		if err := g.emitClone(output, destination+"["+index+"]", source+"["+index+"]", typed.Elt, indent+1); err != nil {
			return err
		}
		fmt.Fprintf(output, "%s}\n", prefix)
	case *ast.Ident:
		if g.isMarked("", typed.Name) {
			fmt.Fprintf(output, "%s%s = *(&%s).DeepCopy()\n", prefix, destination, source)
		} else if g.isStruct("", typed.Name) {
			return fmt.Errorf("struct value %s requires +deepcopy-gen=true", typed.Name)
		} else {
			fmt.Fprintf(output, "%s%s = %s\n", prefix, destination, source)
		}
	case *ast.SelectorExpr:
		if name, packageName, ok := namedType(typed); ok && g.isMarked(packageName, name) {
			fmt.Fprintf(output, "%s%s = *(&%s).DeepCopy()\n", prefix, destination, source)
		} else {
			fmt.Fprintf(output, "%s%s = %s\n", prefix, destination, source)
		}
	case *ast.InterfaceType, *ast.FuncType, *ast.ChanType:
		return fmt.Errorf("unsupported field type %T", expression)
	default:
		return fmt.Errorf("unsupported field type %T", expression)
	}
	return nil
}

func (g *generator) isMarked(packageName, name string) bool {
	pkg := g.current
	if packageName != "" {
		pkg = g.byImport[g.imports[packageName]]
	}
	return pkg != nil && pkg.types[name] != nil && pkg.types[name].marked
}

func (g *generator) isStruct(packageName, name string) bool {
	pkg := g.current
	if packageName != "" {
		pkg = g.byImport[g.imports[packageName]]
	}
	return pkg != nil && pkg.types[name] != nil
}

func (g *generator) next(prefix string) string {
	g.sequence++
	return fmt.Sprintf("%s%d", prefix, g.sequence)
}

func (g *generator) nodeString(node ast.Node) (string, error) {
	switch typed := node.(type) {
	case *ast.Ident:
		return typed.Name, nil
	case *ast.StarExpr:
		inner, err := g.nodeString(typed.X)
		return "*" + inner, err
	case *ast.SelectorExpr:
		prefix, err := g.nodeString(typed.X)
		return prefix + "." + typed.Sel.Name, err
	case *ast.ArrayType:
		element, err := g.nodeString(typed.Elt)
		if err != nil {
			return "", err
		}
		if typed.Len == nil {
			return "[]" + element, nil
		}
		length, err := g.nodeString(typed.Len)
		return "[" + length + "]" + element, err
	case *ast.MapType:
		key, err := g.nodeString(typed.Key)
		if err != nil {
			return "", err
		}
		value, err := g.nodeString(typed.Value)
		return "map[" + key + "]" + value, err
	case *ast.BasicLit:
		return typed.Value, nil
	case *ast.InterfaceType:
		return "interface{}", nil
	default:
		return "", fmt.Errorf("unsupported type expression %T", node)
	}
}

func (g *generator) mustNodeString(node ast.Node) string {
	result, err := g.nodeString(node)
	if err != nil {
		panic(err)
	}
	return result
}

func namedType(expression ast.Expr) (string, string, bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name, "", true
	case *ast.SelectorExpr:
		if packageIdent, ok := typed.X.(*ast.Ident); ok {
			return typed.Sel.Name, packageIdent.Name, true
		}
	}
	return "", "", false
}

func isPrimitive(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}
