package streamer

import (
	"sync"

	"github.com/gopxl/beep"
)

var silence = beep.Silence(-1)

type Channel struct {
	queue []beep.Streamer
	l     sync.Mutex
}

func (ch *Channel) Stream(samples [][2]float64) (n int, ok bool) {
    ch.l.Lock()
    defer ch.l.Unlock()

	t := 0
	l := len(samples)
	for {
		n, ok = ch.current().Stream(samples[t:])
		if !ok {
            ch.next()
		}
		t += n
		if t >= l {
			return l, true
		}
	}
}

func (ch *Channel) Err() error {
	return nil
}

func (ch *Channel) Discard() {
	ch.l.Lock()
	defer ch.l.Unlock()
	ch.queue = nil
}

func (ch *Channel) Add(streams ...beep.Streamer) {
	ch.l.Lock()
	defer ch.l.Unlock()
	ch.queue = append(ch.queue, streams...)
}

func (ch *Channel) current() beep.Streamer {
    if len(ch.queue) == 0 {
        return silence
    }
    return ch.queue[0]
}

func (ch *Channel) next() {
    if len(ch.queue) == 0 {
        return
    }
    ch.queue = ch.queue[1:]
}

