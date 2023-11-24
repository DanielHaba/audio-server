package streamer

import (
	"sync"

	"github.com/gopxl/beep"
)

type Library struct {
    data map[string]*beep.Buffer
    l sync.RWMutex
}



func (m *Library) Has(key string) bool {
    m.l.RLock()
    defer m.l.RUnlock()

    if m.data != nil {
        _, ok := m.data[key]

        return ok
    }
    return false
}

func (m *Library) Get(key string) (*beep.Buffer, bool) {
    m.l.RLock()
    defer m.l.RUnlock()

    if m.data != nil {
        v, ok := m.data[key]

        return v, ok
    }
    return nil, false
}

func (m *Library) Stream(key string) (beep.Streamer, bool) {
    if v, ok := m.Get(key); ok {
        return v.Streamer(0, v.Len()), true
    }
    return nil, false
}

func (m *Library) Set(key string, s *beep.Buffer) {
    m.l.Lock()
    defer m.l.Unlock()

    if m.data == nil {
        m.data = make(map[string]*beep.Buffer)
    }
    m.data[key] = s
}

func (m *Library) Insert(key string, fn func() *beep.Buffer) *beep.Buffer {
    m.l.Lock()
    defer m.l.Unlock()

    if m.data == nil {
        m.data = make(map[string]*beep.Buffer)
    }
    if _, ok := m.data[key]; !ok {
        m.data[key] = fn()
    }
    return m.data[key]
}
