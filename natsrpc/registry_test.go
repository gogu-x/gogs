package natsrpc

import "testing"

func TestSubjectForBuiltinModules(t *testing.T) {
	cases := []struct {
		module string
		id     string
		nodeID string
		want   string
	}{
		{GameNats, "1", "2", "game:1:2"},
		{ModuleGate, "3", "", "gate.out.3"},
		{ModuleCross, "4", "", "cross.4"},
		{ModuleDeliver, "5", "", "platform.deliver.5"},
	}
	for _, c := range cases {
		got, err := subjectFor(c.module, c.id, c.nodeID)
		if err != nil {
			t.Fatalf("subjectFor(%s,%s,%s) error: %v", c.module, c.id, c.nodeID, err)
		}
		if got != c.want {
			t.Errorf("subjectFor(%s,%s,%s) = %q, want %q", c.module, c.id, c.nodeID, got, c.want)
		}
	}
}

func TestSubjectForUnknownModule(t *testing.T) {
	if _, err := subjectFor("no-such-module", "1", "1"); err == nil {
		t.Fatal("expected error for unregistered module, got nil")
	}
}

func TestRegisterModuleCustom(t *testing.T) {
	RegisterModule("custom-svc", func(id, nodeID string) string {
		return "custom." + id + "." + nodeID
	})
	got, err := subjectFor("custom-svc", "42", "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "custom.42.7"; got != want {
		t.Errorf("subjectFor custom-svc = %q, want %q", got, want)
	}
}
