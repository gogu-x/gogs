package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDiscoverStructs 验证同文件多个结构体按指令位置生成，手动运行仅扫描标记结构体。
func TestDiscoverStructs(t *testing.T) {
	dir := t.TempDir()
	source := `package sample
// First 测试结构体。
//go:generate accessor-gen
type First struct { Value int }
// Second 测试结构体。
//go:generate accessor-gen
type Second struct { Name string }
type Unmarked struct { Enabled bool }
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		file string
		line int
		want []string
	}{
		{"model.go", 3, []string{"First"}},
		{"model.go", 6, []string{"Second"}},
		{"", 0, []string{"First", "Second"}},
	}
	for _, tc := range cases {
		names, err := discoverStructs(dir, tc.file, tc.line)
		if err != nil || !reflect.DeepEqual(names, tc.want) {
			t.Fatalf("识别错误: names=%v err=%v", names, err)
		}
	}
	names, err := discoverStructs(dir, "model.go", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := generateAccessors(dir, names); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "first_accessors.go"))
	if err != nil || !strings.Contains(string(data), "GetValue") {
		t.Fatal("未生成对应结构体访问方法")
	}
	if _, err := os.Stat(filepath.Join(dir, "second_accessors.go")); !os.IsNotExist(err) {
		t.Fatal("生成了另一条指令的结构体")
	}
	if err := generateAccessors(dir, []string{"Unmarked"}); err != nil {
		t.Fatal("显式指定结构体不兼容", err)
	}
	if _, err := discoverStructs(dir, "model.go", 1); err == nil {
		t.Fatal("未关联结构体应报错")
	}
}
