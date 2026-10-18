// Package event provides common event messages and Actor delivery helpers.
package event

import (
	"fmt"
	"time"

	"github.com/gogu-x/gogs/pb/sspb"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/tlog"
	"google.golang.org/protobuf/proto"
)

const (
	// TypeServerLifecycle identifies server lifecycle events.
	TypeServerLifecycle int32 = 1

	// NameServerOpened identifies the event sent after server startup.
	NameServerOpened = "server.opened"
	// NameServerClosing identifies the event sent before server shutdown.
	NameServerClosing = "server.closing"

	eventDeliveryTimeout = 30 * time.Second
)

// New constructs a generic event message.
func New(name string, eventType int32, args map[string]int32) *sspb.Event {
	return &sspb.Event{Name: name, EventTyp: eventType, Args: args}
}

// Clone copies an event so separate Actor mailboxes do not share its mutable args map.
func Clone(message *sspb.Event) *sspb.Event {
	if message == nil {
		return nil
	}
	return proto.Clone(message).(*sspb.Event)
}

// NewServerOpened constructs the server startup event.
func NewServerOpened() *sspb.Event {
	return New(NameServerOpened, TypeServerLifecycle, nil)
}

// NewServerClosing constructs the server shutdown event.
func NewServerClosing() *sspb.Event {
	return New(NameServerClosing, TypeServerLifecycle, nil)
}

// IsServerLifecycle reports whether message belongs to the server lifecycle event type.
func IsServerLifecycle(message interface{}) bool {
	event, ok := message.(*sspb.Event)
	return ok && event != nil && event.GetEventTyp() == TypeServerLifecycle
}

// IsServerOpened reports whether message is the server startup event.
func IsServerOpened(message interface{}) bool {
	event, ok := message.(*sspb.Event)
	return ok && event != nil && event.GetEventTyp() == TypeServerLifecycle && event.GetName() == NameServerOpened
}

// IsServerClosing reports whether message is the server shutdown event.
func IsServerClosing(message interface{}) bool {
	event, ok := message.(*sspb.Event)
	return ok && event != nil && event.GetEventTyp() == TypeServerLifecycle && event.GetName() == NameServerClosing
}

// SendToActors sends an event to each named Actor and waits for every response.
func SendToActors(system *tree.Tree, message *sspb.Event, actorNames []string) error {
	if system == nil || message == nil {
		err := fmt.Errorf("event message or actor system is nil")
		tlog.Log.Error("[event/SendToActors] 事件投递参数无效, actorCount=%v err=%v", len(actorNames), err)
		return err
	}
	eventName := message.GetName()
	var deliveryErr error
	for _, actorName := range actorNames {
		pid, ok := system.Lookup(actorName)
		if !ok {
			err := fmt.Errorf("actor %s was not found for event %q", actorName, eventName)
			tlog.Log.Error("[event/SendToActors] 事件投递目标 Actor 不存在, actor=%v event=%v err=%v", actorName, eventName, err)
			if deliveryErr == nil {
				deliveryErr = err
			}
			continue
		}
		if _, err := system.Request(pid, Clone(message)).AwaitTimeout(eventDeliveryTimeout); err != nil {
			tlog.Log.Error("[event/SendToActors] 等待 Actor 处理事件失败, actor=%v event=%v err=%v", actorName, eventName, err)
			if deliveryErr == nil {
				deliveryErr = err
			}
		}
	}
	return deliveryErr
}
