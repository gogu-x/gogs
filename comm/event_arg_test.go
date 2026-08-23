package comm

import (
	"reflect"
	"testing"
)

func TestArg(t *testing.T) {
	var arg Arg
	arg.Set("uid", int64(42))
	arg.Set("name", "alice")

	if got, ok := arg.Get("uid"); !ok || got != int64(42) {
		t.Fatalf("Get(uid) = %v, %v; want 42, true", got, ok)
	}
	if !arg.Has("name") || arg.Len() != 2 {
		t.Fatalf("Has(name) = %v, Len() = %d; want true, 2", arg.Has("name"), arg.Len())
	}
	if !arg.Delete("name") || arg.Delete("name") {
		t.Fatal("Delete should report whether the key existed")
	}

	values := arg.Values()
	values["uid"] = int64(7)
	if got, _ := arg.Get("uid"); got != int64(42) {
		t.Fatalf("Values exposed internal map: uid = %v, want 42", got)
	}

	clone := arg.Clone()
	clone.Set("uid", int64(9))
	if got, _ := arg.Get("uid"); got != int64(42) {
		t.Fatalf("Clone modified original: uid = %v, want 42", got)
	}

	arg.Clear()
	if arg.Len() != 0 {
		t.Fatalf("Len after Clear = %d, want 0", arg.Len())
	}
}

func TestEventDispatchUsesPriorityAndRegistrationOrder(t *testing.T) {
	var event Event
	var calls []string
	args := NewArg()
	args.Set("id", 1001)

	event.Register("login", func(arg *Arg) {
		if got, _ := arg.Get("id"); got != 1001 {
			t.Errorf("first handler received id %v, want 1001", got)
		}
		calls = append(calls, "normal-first")
	})
	event.Register("login", func(*Arg) {
		calls = append(calls, "high")
	}, 10)
	event.RegisterWithPriority("login", 0, func(*Arg) {
		calls = append(calls, "normal-second")
	})

	if got := event.Dispatch("login", args); got != 3 {
		t.Fatalf("Dispatch count = %d, want 3", got)
	}
	if want := []string{"high", "normal-first", "normal-second"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("handler order = %v, want %v", calls, want)
	}
}

func TestEventRegistrationDuringDispatchTakesEffectNextTime(t *testing.T) {
	event := NewEvent()
	calls := 0
	event.Register("tick", func(*Arg) {
		calls++
		event.Register("tick", func(*Arg) { calls++ })
	})

	if got := event.Dispatch("tick", nil); got != 1 || calls != 1 {
		t.Fatalf("first Dispatch = %d calls / %d handlers, want 1 / 1", calls, got)
	}
	if got := event.Dispatch("tick", nil); got != 2 || calls != 3 {
		t.Fatalf("second Dispatch = %d calls / %d handlers, want 3 / 2", calls, got)
	}
}

func TestEventClear(t *testing.T) {
	event := NewEvent()
	event.Register("a", func(*Arg) {})
	event.Register("b", func(*Arg) {})
	event.Clear("a")
	if event.ListenerCount("a") != 0 || event.ListenerCount("b") != 1 {
		t.Fatal("Clear(eventName) should only clear the specified event")
	}
	event.Clear("")
	if event.ListenerCount("b") != 0 {
		t.Fatal("Clear(\"\") should clear all events")
	}
}
