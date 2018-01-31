package claimtools

import "sync"

type syncMapStringError struct {
	sync.RWMutex
	store map[string]error
}

func newSyncMapStringError() *syncMapStringError {
	return &syncMapStringError{store: make(map[string]error)}
}

func (m *syncMapStringError) Store(key string, err error) {
	m.Lock()
	m.store[key] = err
	m.Unlock()
}

func (m *syncMapStringError) Load(key string) (bool, error) {
	m.RLock()
	err, ok := m.store[key]
	m.RUnlock()
	if !ok {
		return false, nil
	}
	return true, err
}
