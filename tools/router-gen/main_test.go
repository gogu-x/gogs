package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedRequestHasNoResult 验证生成器调用无返回值处理器，并拒绝有返回值签名。
func TestGeneratedRequestHasNoResult(t *testing.T) {
	directory := t.TempDir()
	source := `package sample
import (
 "github.com/gogu-x/gogs/pb/cspb/pb_actv"
 "github.com/gogu-x/gogs/pb/cspb/pb_common"
 "google.golang.org/protobuf/proto"
)
type Behavior struct{}
// @actv.request
func (b *Behavior) progress(uid uint64, request *pb_actv.GetProgressReq) {}
`
	file := filepath.Join(directory, "logic.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if err := generate(directory); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(filepath.Join(directory, "actv_routes_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "behavior.progress(uid, request.(*pb_actv.GetProgressReq))") || !strings.Contains(string(output), "handler(behavior, uid, request)") {
		t.Fatal("生成的路由没有调用业务处理器")
	}
	source = strings.Replace(source, ") {}", ") (proto.Message, pb_common.ErrCode) { return nil, pb_common.ErrCode_OK }", 1)
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if err := generate(directory); err == nil {
		t.Fatal("有返回值的处理器应被明确拒绝")
	}
}
