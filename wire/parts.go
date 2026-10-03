package wire

import (
	pb "wayseer.dev/sdk/proto/modulev1"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// MaxPart is the largest RunResponse Parts makes, well under gRPC's 4 MiB message limit.
const MaxPart = 1 << 20

// partOverhead is a RunResponse's bytes besides its change set's contents.
const partOverhead = 16

// Parts splits cs into RunResponses of at most limit bytes, keeping each list's order. An item
// larger than the limit travels alone.
func Parts(snapshot bool, cs *pb.ChangeSet, limit int) []*pb.RunResponse {
	s := splitter{limit: limit - partOverhead, cur: &pb.ChangeSet{}}
	for _, r := range cs.GetRemoves() {
		s.add(protowire.SizeBytes(len(r)), func(p *pb.ChangeSet) { p.Removes = append(p.Removes, r) })
	}
	addAll(&s, cs.GetRemoveEdges(), func(p *pb.ChangeSet, k *pb.EdgeKey) { p.RemoveEdges = append(p.RemoveEdges, k) })
	addAll(&s, cs.GetUpserts(), func(p *pb.ChangeSet, e *pb.Entity) { p.Upserts = append(p.Upserts, e) })
	addAll(&s, cs.GetEdges(), func(p *pb.ChangeSet, e *pb.Edge) { p.Edges = append(p.Edges, e) })
	addAll(&s, cs.GetEvents(), func(p *pb.ChangeSet, e *pb.Event) { p.Events = append(p.Events, e) })
	s.parts = append(s.parts, s.cur)

	out := make([]*pb.RunResponse, len(s.parts))
	for i, p := range s.parts {
		out[i] = &pb.RunResponse{Snapshot: snapshot, ChangeSet: p, Continued: i < len(s.parts)-1}
	}
	return out
}

// splitter fills change sets up to a byte limit.
type splitter struct {
	limit, size int
	cur         *pb.ChangeSet
	parts       []*pb.ChangeSet
}

// add puts an item of n encoded bytes (without its tag) into the current part, or a new one.
func (s *splitter) add(n int, put func(*pb.ChangeSet)) {
	n++ // every ChangeSet field's tag is one byte
	if s.size > 0 && s.size+n > s.limit {
		s.parts = append(s.parts, s.cur)
		s.cur, s.size = &pb.ChangeSet{}, 0
	}
	put(s.cur)
	s.size += n
}

func addAll[M proto.Message](s *splitter, items []M, put func(*pb.ChangeSet, M)) {
	for _, m := range items {
		s.add(protowire.SizeBytes(proto.Size(m)), func(p *pb.ChangeSet) { put(p, m) })
	}
}

// Joiner reassembles the parts of each change set Run sends.
type Joiner struct {
	cur      *pb.ChangeSet
	snapshot bool
}

// Add takes the next part, and returns the whole change set when r is its last part.
func (j *Joiner) Add(r *pb.RunResponse) (snapshot bool, cs *pb.ChangeSet, done bool) {
	if j.cur == nil {
		j.cur, j.snapshot = &pb.ChangeSet{}, r.GetSnapshot()
	}
	p := r.GetChangeSet()
	j.cur.Removes = append(j.cur.Removes, p.GetRemoves()...)
	j.cur.RemoveEdges = append(j.cur.RemoveEdges, p.GetRemoveEdges()...)
	j.cur.Upserts = append(j.cur.Upserts, p.GetUpserts()...)
	j.cur.Edges = append(j.cur.Edges, p.GetEdges()...)
	j.cur.Events = append(j.cur.Events, p.GetEvents()...)
	if r.GetContinued() {
		return false, nil, false
	}
	cs, snapshot = j.cur, j.snapshot
	j.cur = nil
	return snapshot, cs, true
}
