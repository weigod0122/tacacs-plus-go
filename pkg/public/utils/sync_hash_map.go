package utils

import "sync"

// TypedSyncMap 是一个带 RWMutex 保护的泛型 map，免去调用方的类型断言开销。
// 与 sync.Map 不同：实现简单（map + RWMutex），读多写少场景性能接近 sync.Map，
// 但接口更直观，且对 K/V 类型有静态保证。
type TypedSyncMap[K comparable, V any] struct {
	rw   sync.RWMutex
	data map[K]V
}

// NewTypedSyncMap 构造一个空的 TypedSyncMap。
func NewTypedSyncMap[K comparable, V any]() *TypedSyncMap[K, V] {
	return &TypedSyncMap[K, V]{data: make(map[K]V)}
}

// Get 返回 (值, 是否存在)。
func (m *TypedSyncMap[K, V]) Get(k K) (V, bool) {
	m.rw.RLock()
	v, ok := m.data[k]
	m.rw.RUnlock()
	return v, ok
}

// Set 写入键值。
func (m *TypedSyncMap[K, V]) Set(k K, v V) {
	m.rw.Lock()
	m.data[k] = v
	m.rw.Unlock()
}

// Delete 删除键。
func (m *TypedSyncMap[K, V]) Delete(k K) {
	m.rw.Lock()
	delete(m.data, k)
	m.rw.Unlock()
}

// Len 当前元素个数。
func (m *TypedSyncMap[K, V]) Len() int {
	m.rw.RLock()
	n := len(m.data)
	m.rw.RUnlock()
	return n
}

// Each 在读锁下遍历所有键值。回调内禁止再调用本 map 的写方法，否则会死锁。
func (m *TypedSyncMap[K, V]) Each(cb func(K, V)) {
	m.rw.RLock()
	defer m.rw.RUnlock()
	for k, v := range m.data {
		cb(k, v)
	}
}
