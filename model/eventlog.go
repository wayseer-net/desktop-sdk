package model

import "iter"

// eventChunk is the unit in which the event log grows and forgets.
const eventChunk = 256

type chunk = [eventChunk]Event

// eventLog is append-only: filled slots never change, so snapshots share chunks safely.
type eventLog struct {
	chunks []*chunk
	last   int    // filled slots in the last chunk
	first  uint64 // the number of the oldest event kept
	cap    int
}

// eventView is a snapshot's fixed window onto the log.
type eventView struct {
	chunks []*chunk
	last   int
	first  uint64
}

func (l *eventLog) append(e Event) {
	if len(l.chunks) == 0 || l.last == eventChunk {
		l.chunks = append(l.chunks, new(chunk))
		l.last = 0
	}
	l.chunks[len(l.chunks)-1][l.last] = e
	l.last++
	// Only reslice: held snapshots share this backing array, so its slots must not change.
	for (len(l.chunks)-1)*eventChunk >= l.cap && len(l.chunks) > 1 {
		l.chunks = l.chunks[1:]
		l.first += eventChunk
	}
}

func (l *eventLog) view() eventView { return eventView{chunks: l.chunks, last: l.last, first: l.first} }

// bounds are the numbers of the oldest event in view and one past the newest.
func (v eventView) bounds() (first, end uint64) {
	if len(v.chunks) == 0 {
		return v.first, v.first
	}
	return v.first, v.first + uint64((len(v.chunks)-1)*eventChunk+v.last)
}

// at is the event numbered seq, if in view.
func (v eventView) at(seq uint64) (*Event, bool) {
	if first, end := v.bounds(); seq < first || seq >= end {
		return nil, false
	}
	i := seq - v.first
	return &v.chunks[i/eventChunk][i%eventChunk], true
}

func (v eventView) all() iter.Seq[*Event] {
	return func(yield func(*Event) bool) {
		for i, c := range v.chunks {
			n := eventChunk
			if i == len(v.chunks)-1 {
				n = v.last
			}
			for j := range n {
				if !yield(&c[j]) {
					return
				}
			}
		}
	}
}
