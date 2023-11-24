package streamer

import (
	"sync"

	"github.com/gopxl/beep"
)

type Mixer struct {
	streams map[string]beep.Streamer
	l     sync.RWMutex
}

func (m *Mixer) Has(key string) bool {
    m.l.RLock()
    defer m.l.RUnlock()

    if m.streams != nil {
        _, ok := m.streams[key]

        return ok
    }
    return false
}

func (m *Mixer) Get(key string) (beep.Streamer, bool) {
    m.l.RLock()
    defer m.l.RUnlock()

    if m.streams != nil {
        v, ok := m.streams[key]

        return v, ok
    }
    return nil, false
}

func (m *Mixer) Set(key string, s beep.Streamer) {
    m.l.Lock()
    defer m.l.Unlock()

    if m.streams == nil {
        m.streams = make(map[string]beep.Streamer)
    }
    m.streams[key] = s
}

func (m *Mixer) Insert(key string, fn func() beep.Streamer) beep.Streamer {
    m.l.Lock()
    defer m.l.Unlock()

    if m.streams == nil {
        m.streams = make(map[string]beep.Streamer)
    }
    if _, ok := m.streams[key]; !ok {
        m.streams[key] = fn()
    }
    return m.streams[key]
}


func (m *Mixer) Stream(samples [][2]float64) (int, bool) {
	m.l.Lock()
	defer m.l.Unlock()

	var (
		tmp [512][2]float64
        t int
	)
	for len(samples) > 0 {
		l := len(tmp)
		if l > len(samples) {
			l = len(samples)
		}
        for i := range samples[:l] {
			samples[i] = [2]float64{}
		}
		for k, ch := range m.streams {
			n, ok := ch.Stream(tmp[:l])
			for i, v := range tmp[:n] {
				samples[i][0] += v[0]
				samples[i][1] += v[1]
			}
			if !ok {
				delete(m.streams, k)
			}
		}
        samples = samples[l:]
        t += l
	}
	return t, true
}

func (m *Mixer) Err() error {
	return nil
}
