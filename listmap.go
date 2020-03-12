package main

import (
	"container/list"
	"fmt"
)

// ListMap is an ordered map with some extra useful methods
type ListMap struct {
	entries map[interface{}]listMapValue
	keys    *list.List
}

// listMapValue represents the value of a map entry
type listMapValue struct {
	val     interface{}
	element *list.Element
}

// ListMapEntry represents a map key value pair entry
type ListMapEntry struct {
	Key, Value interface{}
}

// New creates a new ListMap and returns the ListMap pointer
func NewListMap() *ListMap { return new(ListMap).Init() }

// Init initializes/clears the ListMap and returns the ListMap pointer
func (t *ListMap) Init() *ListMap {
	t.entries = make(map[interface{}]listMapValue)
	if t.keys == nil {
		t.keys = list.New()
	} else {
		t.keys.Init()
	}
	return t
}

// Put enters a new key value pair into the map and returns the ListMap pointer
func (t *ListMap) Put(key interface{}, value interface{}) *ListMap {
	if v, ok := t.entries[key]; ok {
		t.entries[key] = listMapValue{value, v.element}
	} else {
		t.entries[key] = listMapValue{value, t.keys.PushBack(key)}
	}
	return t
}

// PutEntries enters all entries into the ListMap and returns the ListMap
// pointer
func (t *ListMap) PutEntries(entries []ListMapEntry) *ListMap {
	for _, entry := range entries {
		t.Put(entry.Key, entry.Value)
	}
	return t
}

// Delete deletes the key from the map and returns the ListMap pointer
func (t *ListMap) Delete(key interface{}) *ListMap {
	if _, ok := t.entries[key]; ok {
		t.keys.Remove(t.entries[key].element)
		delete(t.entries, key)
	}
	return t
}

// DeleteAll deletes all the keys from the map and returns the ListMap pointer
func (t *ListMap) DeleteAll(keys []interface{}) *ListMap {
	for _, k := range keys {
		t.Delete(k)
	}
	return t
}

// Keys creates and returns a slice of all the map keys
func (t *ListMap) Keys() []interface{} {
	keys := make([]interface{}, t.keys.Len())
	for e, i := t.keys.Front(), 0; e != nil; e, i = e.Next(), i+1 {
		keys[i] = e.Value
	}
	return keys
}

// String returns a string representing the map entries
func (t *ListMap) String() string {
	return fmt.Sprint(t.Entries())
}

// Values creates and returns a slice of all the map values
func (t *ListMap) Values() []interface{} {
	vals := make([]interface{}, t.keys.Len())
	for e, i := t.keys.Front(), 0; e != nil; e, i = e.Next(), i+1 {
		vals[i] = t.entries[e.Value].val
	}
	return vals
}

// Entries creates and returns a slice of all the map key value pair entries
func (t *ListMap) Entries() []ListMapEntry {
	entries := make([]ListMapEntry, t.keys.Len())
	for e, i := t.keys.Front(), 0; e != nil; e, i = e.Next(), i+1 {
		entries[i] = ListMapEntry{Key: e.Value, Value: t.entries[e.Value].val}
	}
	return entries
}

// Get gets and returns the value for the specified search key
func (t *ListMap) Get(key interface{}) interface{} {
	if val, ok := t.entries[key]; ok {
		return val.val
	}
	return nil
}

// Contains returns true if the map contains the key
func (t *ListMap) Contains(key interface{}) bool {
	_, ok := t.entries[key]
	return ok
}

// ContainsAll returns true if the map contains all the keys
func (t *ListMap) ContainsAll(keys []interface{}) bool {
	for _, key := range keys {
		if !t.Contains(key) {
			return false
		}
	}
	return true
}

// ContainsAny returns true if the map contains any of the keys
func (t *ListMap) ContainsAny(keys []interface{}) bool {
	for _, key := range keys {
		if t.Contains(key) {
			return true
		}
	}
	return false
}

// Len returns the number of map entries
func (t *ListMap) Len() int {
	return t.keys.Len()
}

// Head returns the first value of the ordered map
func (t *ListMap) Head() interface{} {
	if t.NotEmpty() {
		return t.entries[t.keys.Front().Value].val
	}
	return nil
}

// Tail returns the last value of the ordered map
func (t *ListMap) Tail() interface{} {
	if t.NotEmpty() {
		return t.entries[t.keys.Back().Value].val
	}
	return nil
}

// Pop deletes the last map entry and returns its value
func (t *ListMap) Pop() interface{} {
	if t.NotEmpty() {
		key := t.keys.Remove(t.keys.Back())
		val, ok := t.entries[key]
		delete(t.entries, key)
		if ok {
			return val.val
		}
	}
	return nil
}

// Pull deletes the first map entry and returns its value
func (t *ListMap) Pull() interface{} {
	if t.NotEmpty() {
		key := t.keys.Remove(t.keys.Front())
		val, ok := t.entries[key]
		delete(t.entries, key)
		if ok {
			return val.val
		}
	}
	return nil
}

// Empty returns true if the ListMap is empty
func (t *ListMap) Empty() bool {
	return t.keys.Len() == 0
}

// NotEmpty returns true if the ListMap is not empty
func (t *ListMap) NotEmpty() bool {
	return t.keys.Len() > 0
}
