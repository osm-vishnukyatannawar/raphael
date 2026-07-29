package secret

import "sync"

// Memory is an in-process Store for tests. It is safe for concurrent use so it
// can back parallel tests.
type Memory struct {
	mu   sync.Mutex
	vals map[string]string

	// FailSet makes Set return an error, standing in for a broken or absent
	// keyring so the degraded path can be exercised.
	FailSet error
}

func NewMemory() *Memory {
	return &Memory{vals: map[string]string{}}
}

func (m *Memory) Set(key, value string) error {
	if m.FailSet != nil {
		return m.FailSet
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.vals == nil {
		m.vals = map[string]string{}
	}
	m.vals[key] = value

	return nil
}

func (m *Memory) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vals[key]
	if !ok {
		return "", ErrNotFound
	}

	return v, nil
}

func (m *Memory) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.vals, key)

	return nil
}

// Has reports whether a key is present, for assertions in tests.
func (m *Memory) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.vals[key]

	return ok
}
