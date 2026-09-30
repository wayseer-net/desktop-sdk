package wire

import (
	"mindseye/pkg/sdk"
	pb "mindseye/pkg/sdk/proto/modulev1"
)

// EncodeChangeSet converts cs to the contract; nil encodes as empty.
func EncodeChangeSet(cs *sdk.ChangeSet) *pb.ChangeSet {
	if cs == nil {
		return &pb.ChangeSet{}
	}
	return &pb.ChangeSet{
		Removes:     convertStrings[string](cs.Removes),
		RemoveEdges: mapSlice(cs.RemoveEdges, encodeEdgeKey),
		Upserts:     mapSlice(cs.Upserts, encodeEntity),
		Edges:       mapSlice(cs.Edges, encodeEdge),
		Events:      mapSlice(cs.Events, encodeEvent),
	}
}

// DecodeChangeSet converts cs from the contract.
func DecodeChangeSet(cs *pb.ChangeSet) *sdk.ChangeSet {
	return &sdk.ChangeSet{
		Removes:     convertStrings[sdk.EntityRef](cs.GetRemoves()),
		RemoveEdges: mapSlice(cs.GetRemoveEdges(), decodeEdgeKey),
		Upserts:     mapSlice(cs.GetUpserts(), decodeEntity),
		Edges:       mapSlice(cs.GetEdges(), decodeEdge),
		Events:      mapSlice(cs.GetEvents(), decodeEvent),
	}
}

func encodeEntity(e sdk.Entity) *pb.Entity {
	return &pb.Entity{
		Ref: string(e.Ref), Kind: string(e.Kind), Name: e.Name, Attrs: encodeAttrs(e.Attrs),
		Status: pb.StatusLevel(e.Status.Level), Reason: e.Status.Reason, Tags: e.Tags,
		Source: string(e.Source), Seen: encodeTime(e.Seen),
	}
}

func decodeEntity(e *pb.Entity) sdk.Entity {
	return sdk.Entity{
		Ref: sdk.EntityRef(e.GetRef()), Kind: sdk.Kind(e.GetKind()), Name: e.GetName(), Attrs: decodeAttrs(e.GetAttrs()),
		Status: sdk.Status{Level: sdk.StatusLevel(e.GetStatus()), Reason: e.GetReason()},
		Tags:   convertStrings[string](e.GetTags()), Source: sdk.ModuleID(e.GetSource()), Seen: decodeTime(e.Seen),
	}
}

func encodeEdgeKey(k sdk.EdgeKey) *pb.EdgeKey {
	return &pb.EdgeKey{From: string(k.From), To: string(k.To), Rel: string(k.Rel)}
}

func decodeEdgeKey(k *pb.EdgeKey) sdk.EdgeKey {
	return sdk.EdgeKey{From: sdk.EntityRef(k.GetFrom()), To: sdk.EntityRef(k.GetTo()), Rel: sdk.Relation(k.GetRel())}
}

func encodeEdge(e sdk.Edge) *pb.Edge {
	return &pb.Edge{
		From: string(e.From), To: string(e.To), Rel: string(e.Rel), Weight: e.Weight,
		Attrs: encodeAttrs(e.Attrs), Source: string(e.Source), Traffic: encodeTraffic(e.Traffic),
	}
}

func decodeEdge(e *pb.Edge) sdk.Edge {
	return sdk.Edge{
		From: sdk.EntityRef(e.GetFrom()), To: sdk.EntityRef(e.GetTo()), Rel: sdk.Relation(e.GetRel()),
		Weight: e.GetWeight(), Attrs: decodeAttrs(e.GetAttrs()), Source: sdk.ModuleID(e.GetSource()),
		Traffic: decodeTraffic(e.GetTraffic()),
	}
}

// encodeTraffic leaves unknown traffic absent, as a 1.3 module sends it.
func encodeTraffic(t sdk.Traffic) *pb.Traffic {
	if t == (sdk.Traffic{}) {
		return nil
	}
	return &pb.Traffic{Rate: t.Rate, Unit: t.Unit.String()}
}

// unknownTrafficUnit stands for a unit this app cannot read; validation refuses it.
const unknownTrafficUnit sdk.TrafficUnit = 255

func decodeTraffic(t *pb.Traffic) sdk.Traffic {
	if t == nil {
		return sdk.Traffic{}
	}
	u, err := sdk.ParseTrafficUnit(t.GetUnit())
	if err != nil {
		u = unknownTrafficUnit
	}
	return sdk.Traffic{Rate: t.GetRate(), Unit: u}
}

func encodeEvent(e sdk.Event) *pb.Event {
	return &pb.Event{
		Id: e.ID, Entity: string(e.Entity), At: encodeTime(e.At), Severity: pb.Severity(e.Severity),
		Kind: e.Kind, Message: e.Message, Fields: encodeAttrs(e.Fields), Source: string(e.Source),
	}
}

func decodeEvent(e *pb.Event) sdk.Event {
	return sdk.Event{
		ID: e.GetId(), Entity: sdk.EntityRef(e.GetEntity()), At: decodeTime(e.At), Severity: sdk.Severity(e.GetSeverity()),
		Kind: e.GetKind(), Message: e.GetMessage(), Fields: decodeAttrs(e.GetFields()), Source: sdk.ModuleID(e.GetSource()),
	}
}
