// Command activity-router-gen 根据活动处理函数上的注释生成类型路由。
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	requestAnnotation = "@activity.request"
	eventAnnotation   = "@activity.event"
)

type route struct {
	method      string
	behavior    string
	requestType ast.Expr
}

type eventRoute struct {
	method    string
	behavior  string
	eventType int32
	eventName string
}

type eventKey struct {
	eventType int32
	eventName string
}

func main() {
	directory := flag.String("dir", ".", "待扫描的活动模板 Go 包目录")
	flag.Parse()
	if err := generate(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read activity template directory: %w", err)
	}

	fset := token.NewFileSet()
	packageName := ""
	imports := make(map[string]string)
	routes := make([]route, 0)
	eventRoutes := make([]eventRoute, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path.Join(directory, name), nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		if packageName == "" {
			packageName = file.Name.Name
		} else if packageName != file.Name.Name {
			return fmt.Errorf("activity template package %s contains package %s", directory, file.Name.Name)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("decode import in %s: %w", name, err)
			}
			alias := path.Base(importPath)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			imports[alias] = importPath
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if hasAnnotation(function.Doc, requestAnnotation) {
				parsed, err := parseRoute(function)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				routes = append(routes, parsed)
			}
			if annotationValue, found := findAnnotation(function.Doc, eventAnnotation); found {
				parsed, err := parseEventRoute(fset, function, annotationValue)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				eventRoutes = append(eventRoutes, parsed)
			}
		}
	}
	if packageName == "" {
		return fmt.Errorf("no Go source files found in %s", directory)
	}
	if len(routes) == 0 && len(eventRoutes) == 0 {
		return fmt.Errorf("no functions annotated with %s or %s found in %s", requestAnnotation, eventAnnotation, directory)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].method < routes[j].method })
	sort.Slice(eventRoutes, func(i, j int) bool {
		if eventRoutes[i].eventType == eventRoutes[j].eventType {
			return eventRoutes[i].eventName < eventRoutes[j].eventName
		}
		return eventRoutes[i].eventType < eventRoutes[j].eventType
	})
	behaviorType := ""
	seenRequests := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if behaviorType == "" {
			behaviorType = route.behavior
		}
		if route.behavior != behaviorType {
			return fmt.Errorf("annotated functions in %s must use the same behavior receiver type", directory)
		}
		requestType := nodeString(fset, route.requestType)
		if _, found := seenRequests[requestType]; found {
			return fmt.Errorf("duplicate activity request route for %s", requestType)
		}
		seenRequests[requestType] = struct{}{}
	}
	seenEvents := make(map[eventKey]struct{}, len(eventRoutes))
	for _, route := range eventRoutes {
		if behaviorType == "" {
			behaviorType = route.behavior
		}
		if route.behavior != behaviorType {
			return fmt.Errorf("annotated functions in %s must use the same behavior receiver type", directory)
		}
		key := eventKey{eventType: route.eventType, eventName: route.eventName}
		if _, found := seenEvents[key]; found {
			return fmt.Errorf("duplicate activity event route for type %d name %q", key.eventType, key.eventName)
		}
		seenEvents[key] = struct{}{}
	}
	usedAliases := make(map[string]string)
	// 生成文件需要处理函数使用的请求和事件类型。
	for _, route := range routes {
		collectSignatureImports(fset, route, imports, usedAliases)
	}
	if len(eventRoutes) > 0 {
		const eventImportPath = "github.com/gogu-x/gogs/pb/sspb"
		if imports["sspb"] != eventImportPath {
			return fmt.Errorf("event handlers in %s must import %q as sspb", directory, eventImportPath)
		}
		usedAliases["sspb"] = eventImportPath
	}
	if len(routes) > 0 {
		if existing, ok := usedAliases["reflect"]; ok && existing != "reflect" {
			return fmt.Errorf("activity request type import alias reflect conflicts with standard library")
		}
		usedAliases["reflect"] = "reflect"
		const protoImportPath = "google.golang.org/protobuf/proto"
		if existing, ok := usedAliases["proto"]; ok && existing != protoImportPath {
			return fmt.Errorf("activity request type import alias proto conflicts with %s", protoImportPath)
		}
		usedAliases["proto"] = protoImportPath
	}

	source, err := render(fset, packageName, behaviorType, routes, eventRoutes, usedAliases)
	if err != nil {
		return err
	}
	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format generated activity routes: %w\n%s", err, source)
	}
	outputPath := path.Join(directory, "activity_routes_gen.go")
	if err := os.WriteFile(outputPath, formatted, 0o644); err != nil {
		return fmt.Errorf("write generated activity routes: %w", err)
	}
	return nil
}

func hasAnnotation(group *ast.CommentGroup, marker string) bool {
	_, found := findAnnotation(group, marker)
	return found
}

func findAnnotation(group *ast.CommentGroup, marker string) (string, bool) {
	if group == nil {
		return "", false
	}
	for _, comment := range group.List {
		value := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		if value == marker {
			return "", true
		}
		if strings.HasPrefix(value, marker+" ") {
			return strings.TrimSpace(strings.TrimPrefix(value, marker)), true
		}
	}
	return "", false
}

func parseRoute(function *ast.FuncDecl) (route, error) {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return route{}, fmt.Errorf("annotated function %s must have a pointer behavior receiver", function.Name.Name)
	}
	receiverPointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return route{}, fmt.Errorf("annotated function %s must use a pointer behavior receiver", function.Name.Name)
	}
	receiver, ok := receiverPointer.X.(*ast.Ident)
	if !ok {
		return route{}, fmt.Errorf("annotated function %s must have a named behavior receiver", function.Name.Name)
	}
	arguments := flattenParameters(function.Type.Params)
	if len(arguments) != 2 || function.Type.Results != nil && len(function.Type.Results.List) != 0 {
		return route{}, fmt.Errorf("annotated function %s must have parameters (uid uint64, request *T) and no results", function.Name.Name)
	}
	if arguments[0].name != "uid" {
		return route{}, fmt.Errorf("annotated function %s parameter 1 must be named uid", function.Name.Name)
	}
	if ident, ok := arguments[0].typeExpr.(*ast.Ident); !ok || ident.Name != "uint64" {
		return route{}, fmt.Errorf("annotated function %s uid parameter must be uint64", function.Name.Name)
	}
	if arguments[1].name != "request" {
		return route{}, fmt.Errorf("annotated function %s parameter 2 must be named request", function.Name.Name)
	}
	_, ok = arguments[1].typeExpr.(*ast.StarExpr)
	if !ok {
		return route{}, fmt.Errorf("annotated function %s request parameter must be a pointer type", function.Name.Name)
	}
	return route{
		method:      function.Name.Name,
		behavior:    receiver.Name,
		requestType: arguments[1].typeExpr,
	}, nil
}

func parseEventRoute(fset *token.FileSet, function *ast.FuncDecl, annotationValue string) (eventRoute, error) {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return eventRoute{}, fmt.Errorf("annotated function %s must have a pointer behavior receiver", function.Name.Name)
	}
	receiverPointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return eventRoute{}, fmt.Errorf("annotated function %s must use a pointer behavior receiver", function.Name.Name)
	}
	receiver, ok := receiverPointer.X.(*ast.Ident)
	if !ok {
		return eventRoute{}, fmt.Errorf("annotated function %s must have a named behavior receiver", function.Name.Name)
	}
	arguments := flattenParameters(function.Type.Params)
	if len(arguments) != 1 || (function.Type.Results != nil && len(function.Type.Results.List) != 0) {
		return eventRoute{}, fmt.Errorf("annotated event function %s must have one parameter and no results", function.Name.Name)
	}
	if arguments[0].name != "event" || nodeString(fset, arguments[0].typeExpr) != "*sspb.Event" {
		return eventRoute{}, fmt.Errorf("annotated event function %s must accept event *sspb.Event", function.Name.Name)
	}
	fields := strings.Fields(annotationValue)
	if len(fields) != 2 {
		return eventRoute{}, fmt.Errorf("annotated event function %s must specify an event type and name", function.Name.Name)
	}
	eventType, err := strconv.ParseInt(fields[0], 10, 32)
	if err != nil {
		return eventRoute{}, fmt.Errorf("annotated event function %s has invalid event type %q: %w", function.Name.Name, fields[0], err)
	}
	eventName := fields[1]
	if strings.HasPrefix(eventName, "\"") {
		eventName, err = strconv.Unquote(eventName)
		if err != nil {
			return eventRoute{}, fmt.Errorf("annotated event function %s has invalid event name: %w", function.Name.Name, err)
		}
	}
	if eventName == "" {
		return eventRoute{}, fmt.Errorf("annotated event function %s must specify a non-empty event name", function.Name.Name)
	}
	return eventRoute{
		method:    function.Name.Name,
		behavior:  receiver.Name,
		eventType: int32(eventType),
		eventName: eventName,
	}, nil
}

type namedType struct {
	name     string
	typeExpr ast.Expr
}

func flattenParameters(fields *ast.FieldList) []namedType {
	if fields == nil {
		return nil
	}
	parameters := make([]namedType, 0)
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			parameters = append(parameters, namedType{typeExpr: field.Type})
			continue
		}
		for _, name := range field.Names {
			parameters = append(parameters, namedType{name: name.Name, typeExpr: field.Type})
		}
	}
	return parameters
}

func nodeString(fset *token.FileSet, node ast.Node) string {
	var output bytes.Buffer
	if err := format.Node(&output, fset, node); err != nil {
		return "<invalid>"
	}
	return output.String()
}

func collectSignatureImports(fset *token.FileSet, route route, imports, used map[string]string) {
	collectImports(fset, route.requestType, imports, used)
}

func collectImports(fset *token.FileSet, expression ast.Expr, imports, used map[string]string) {
	ast.Inspect(expression, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		alias, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if importPath, found := imports[alias.Name]; found {
			used[alias.Name] = importPath
		}
		return true
	})
}

func render(
	fset *token.FileSet,
	packageName string,
	behaviorType string,
	routes []route,
	eventRoutes []eventRoute,
	imports map[string]string,
) ([]byte, error) {
	var source strings.Builder
	fmt.Fprintf(&source, "// Code generated by activity-router-gen. DO NOT EDIT.\n\npackage %s\n\nimport (\n", packageName)
	if len(routes) > 0 {
		source.WriteString("\t\"reflect\"\n")
	}
	aliases := make([]string, 0, len(imports))
	for alias := range imports {
		if alias != "reflect" {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	standardAliases := make([]string, 0, len(aliases))
	externalAliases := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if isStandardImport(imports[alias]) {
			standardAliases = append(standardAliases, alias)
			continue
		}
		externalAliases = append(externalAliases, alias)
	}
	for _, alias := range standardAliases {
		writeImport(&source, alias, imports[alias])
	}
	if len(externalAliases) > 0 {
		source.WriteString("\n")
	}
	for _, alias := range externalAliases {
		writeImport(&source, alias, imports[alias])
	}
	source.WriteString(")\n\n")
	if len(routes) > 0 {
		fmt.Fprintf(&source, "type generatedRequestHandler func(*%s, uint64, proto.Message)\n\n", behaviorType)
		source.WriteString("var generatedRequestHandlers = map[reflect.Type]generatedRequestHandler{\n")
		for _, route := range routes {
			pointerType := route.requestType.(*ast.StarExpr).X
			fmt.Fprintf(&source, "\treflect.TypeOf((*%s)(nil)): func(behavior *%s, uid uint64, request proto.Message) {\n",
				nodeString(fset, pointerType), behaviorType)
			fmt.Fprintf(&source, "\t\tbehavior.%s(uid, request.(%s))\n\t},\n",
				route.method, nodeString(fset, route.requestType))
		}
		source.WriteString("}\n\n")
		fmt.Fprintf(&source, "// HandleRequest 按注释标记生成的请求类型路由调用处理函数，uid 为发起请求的玩家。\nfunc (behavior *%s) HandleRequest(uid uint64, request proto.Message) {\n",
			behaviorType)
		source.WriteString("\tif behavior == nil || request == nil {\n\t\treturn\n\t}\n\thandler, found := generatedRequestHandlers[reflect.TypeOf(request)]\n\tif !found {\n\t\treturn\n\t}\n\thandler(behavior, uid, request)\n}\n\n")
	}
	if len(eventRoutes) > 0 {
		source.WriteString("type generatedActivityEventKey struct {\n\teventType int32\n\tname string\n}\n\n")
		fmt.Fprintf(&source, "type generatedActivityEventHandler func(*%s, *sspb.Event)\n\n", behaviorType)
		source.WriteString("var generatedActivityEventHandlers = map[generatedActivityEventKey]generatedActivityEventHandler{\n")
		for _, route := range eventRoutes {
			fmt.Fprintf(&source, "\t{eventType: %d, name: %q}: func(behavior *%s, event *sspb.Event) {\n",
				route.eventType, route.eventName, behaviorType)
			fmt.Fprintf(&source, "\t\tbehavior.%s(event)\n\t},\n", route.method)
		}
		source.WriteString("}\n\n")
		fmt.Fprintf(&source, "// HandleEvent 按事件类型和名称路由调用模板事件处理函数。\nfunc (behavior *%s) HandleEvent(event *sspb.Event) {\n",
			behaviorType)
		source.WriteString("\tif behavior == nil || event == nil {\n\t\treturn\n\t}\n\tkey := generatedActivityEventKey{eventType: event.GetEventTyp(), name: event.GetName()}\n\thandler, found := generatedActivityEventHandlers[key]\n\tif !found {\n\t\treturn\n\t}\n\thandler(behavior, event)\n}\n")
	}
	return []byte(source.String()), nil
}

func isStandardImport(importPath string) bool {
	firstPathSegment := strings.Split(importPath, "/")[0]
	return !strings.Contains(firstPathSegment, ".")
}

func writeImport(source *strings.Builder, alias, importPath string) {
	if alias == path.Base(importPath) {
		fmt.Fprintf(source, "\t%q\n", importPath)
		return
	}
	fmt.Fprintf(source, "\t%s %q\n", alias, importPath)
}
