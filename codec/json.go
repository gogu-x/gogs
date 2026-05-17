package codec

import (
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// 格式：{"MsgName": {...}}

var JsonCodec = &jsonCodec{
	nameToType: make(map[string]reflect.Type),
}

type jsonCodec struct {
	nameToType map[string]reflect.Type
}

func (c *jsonCodec) register(msg proto.Message) {
	t := reflect.TypeOf(msg)
	c.nameToType[t.Elem().Name()] = t
}

func (c *jsonCodec) Unmarshal(data []byte) (interface{}, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if len(m) != 1 {
		return nil, fmt.Errorf("json: expected single key object")
	}
	for name, raw := range m {
		t, ok := c.nameToType[name]
		if !ok {
			return nil, fmt.Errorf("json: unknown message %q", name)
		}
		msg := reflect.New(t.Elem()).Interface()
		return msg, json.Unmarshal(raw, msg)
	}
	panic("unreachable")
}

func (c *jsonCodec) Marshal(msg interface{}) ([]byte, error) {
	t := reflect.TypeOf(msg)
	name := t.Elem().Name()
	if _, ok := c.nameToType[name]; !ok {
		return nil, fmt.Errorf("json: message %s not registered", name)
	}
	return json.Marshal(map[string]interface{}{name: msg})
}
