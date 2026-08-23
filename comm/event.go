package comm

// EventHandler 是事件订阅者的回调函数。args 可以为 nil。
type EventHandler func(args *Arg)

type registeredHandler struct {
	priority int
	handler  EventHandler
}

// Event 提供事件的注册与同步分发。
//
// Event 不包含并发控制，应由同一 goroutine 使用。处理函数按优先级从高到低执行；
// 相同优先级保持注册顺序。分发开始时会复制监听器列表，因此分发过程中新增的处理函数
// 从下一次分发开始生效。Event 的零值可以直接使用。
type Event struct {
	handlers map[string][]registeredHandler
}

// EventBus 是 Event 的别名，用于强调其作为事件总线的用途。
type EventBus = Event

// NewEvent 创建一个空事件总线。
func NewEvent() *Event {
	return &Event{handlers: make(map[string][]registeredHandler)}
}

// NewEventBus 创建一个空事件总线。
func NewEventBus() *EventBus {
	return NewEvent()
}

// Register 注册 eventName 的处理函数。priority 未指定时默认为 0；指定多个值时仅使用第一个。
// 空事件名或 nil 处理函数会被忽略。
func (e *Event) Register(eventName string, handler EventHandler, priority ...int) {
	value := 0
	if len(priority) > 0 {
		value = priority[0]
	}
	e.RegisterWithPriority(eventName, value, handler)
}

// RegisterWithPriority 注册 eventName 的处理函数。优先级越高，分发时越先执行。
func (e *Event) RegisterWithPriority(eventName string, priority int, handler EventHandler) {
	if e == nil || eventName == "" || handler == nil {
		return
	}
	if e.handlers == nil {
		e.handlers = make(map[string][]registeredHandler)
	}

	handlers := e.handlers[eventName]
	insertAt := len(handlers)
	for i, registered := range handlers {
		if priority > registered.priority {
			insertAt = i
			break
		}
	}
	handlers = append(handlers, registeredHandler{})
	copy(handlers[insertAt+1:], handlers[insertAt:])
	handlers[insertAt] = registeredHandler{priority: priority, handler: handler}
	e.handlers[eventName] = handlers
}

// On 是 Register 的别名。
func (e *Event) On(eventName string, handler EventHandler, priority ...int) {
	e.Register(eventName, handler, priority...)
}

// Dispatch 同步分发 eventName，并返回实际调用的处理函数数量。
func (e *Event) Dispatch(eventName string, args *Arg) int {
	if e == nil || eventName == "" {
		return 0
	}

	handlers := append([]registeredHandler(nil), e.handlers[eventName]...)
	for _, registered := range handlers {
		registered.handler(args)
	}
	return len(handlers)
}

// Emit 是 Dispatch 的别名。
func (e *Event) Emit(eventName string, args *Arg) int {
	return e.Dispatch(eventName, args)
}

// ListenerCount 返回 eventName 当前注册的处理函数数量。
func (e *Event) ListenerCount(eventName string) int {
	if e == nil {
		return 0
	}
	return len(e.handlers[eventName])
}

// Clear 移除 eventName 的所有处理函数。eventName 为空时移除所有处理函数。
func (e *Event) Clear(eventName string) {
	if e == nil {
		return
	}
	if eventName == "" {
		clear(e.handlers)
		return
	}
	delete(e.handlers, eventName)
}
