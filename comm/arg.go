package comm

// Arg 是事件和其他场景共用的键值参数容器。
//
// Arg 不提供并发控制；在多个 goroutine 间共享同一个 Arg 时，调用方应负责同步。
// 零值可以直接使用。
type Arg struct {
	values map[string]any
}

// NewArg 创建一个空参数容器。
func NewArg() *Arg {
	return &Arg{values: make(map[string]any)}
}

// NewArgFromMap 使用 values 的浅拷贝创建参数容器。
func NewArgFromMap(values map[string]any) *Arg {
	arg := NewArg()
	for key, value := range values {
		arg.values[key] = value
	}
	return arg
}

// Set 设置 key 对应的值。Arg 为 nil 时该操作无效。
func (a *Arg) Set(key string, value any) {
	if a == nil {
		return
	}
	if a.values == nil {
		a.values = make(map[string]any)
	}
	a.values[key] = value
}

// Get 返回 key 对应的值及其是否存在。Arg 为 nil 时返回 nil, false。
func (a *Arg) Get(key string) (any, bool) {
	if a == nil {
		return nil, false
	}
	value, ok := a.values[key]
	return value, ok
}

// Has 返回 key 是否存在。
func (a *Arg) Has(key string) bool {
	_, ok := a.Get(key)
	return ok
}

// Delete 删除 key，并返回该 key 是否存在。
func (a *Arg) Delete(key string) bool {
	if a == nil {
		return false
	}
	if _, ok := a.values[key]; !ok {
		return false
	}
	delete(a.values, key)
	return true
}

// Len 返回参数个数。
func (a *Arg) Len() int {
	if a == nil {
		return 0
	}
	return len(a.values)
}

// Clear 删除所有参数。
func (a *Arg) Clear() {
	if a != nil {
		clear(a.values)
	}
}

// Values 返回当前参数的浅拷贝。修改返回的 map 不会影响 Arg。
func (a *Arg) Values() map[string]any {
	values := make(map[string]any)
	if a == nil {
		return values
	}
	for key, value := range a.values {
		values[key] = value
	}
	return values
}

// Clone 返回 Arg 的浅拷贝。
func (a *Arg) Clone() *Arg {
	return NewArgFromMap(a.Values())
}
