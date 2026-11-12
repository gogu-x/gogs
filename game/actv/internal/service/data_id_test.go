package service

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

// TestDataDocumentID 验证独立编号、重复存盘稳定性和加载后编号恢复。
func TestDataDocumentID(t *testing.T) {
	a := &Actv{}
	id := a.dataDocID(1, 7)
	if _, err := bson.ObjectIDFromHex(id); err != nil {
		t.Fatal(err)
	}
	if a.dataDocID(1, 7) != id {
		t.Fatal("重复存盘改变编号")
	}
	if a.dataDocID(2, 7) == id || a.dataDocID(1, 8) == id {
		t.Fatal("不同数据复用编号")
	}
	b := &Actv{}
	b.rememberDataID(&ActvDataDoc{ID: id, Comp: 1, OwnerID: 7})
	if b.dataDocID(1, 7) != id {
		t.Fatal("重载后改变编号")
	}
	if (&Actv{}).dataDocID(1, 7) == id {
		t.Fatal("不同活动复用编号")
	}
}
