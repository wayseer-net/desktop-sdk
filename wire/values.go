package wire

import (
	"mindseye/pkg/sdk"
	pb "mindseye/pkg/sdk/proto/modulev1"
	"time"
)

func encodeValue(v sdk.Value) *pb.Value {
	switch v.Type() {
	case sdk.TypeString:
		return &pb.Value{Value: &pb.Value_StringValue{StringValue: v.Str()}}
	case sdk.TypeNumber:
		return &pb.Value{Value: &pb.Value_NumberValue{NumberValue: v.Num()}, Unit: string(v.Unit())}
	case sdk.TypeBool:
		return &pb.Value{Value: &pb.Value_BoolValue{BoolValue: v.Bool()}}
	case sdk.TypeTime:
		return &pb.Value{Value: &pb.Value_TimeValue{TimeValue: v.Time().UnixNano()}}
	case sdk.TypeList:
		return &pb.Value{Value: &pb.Value_ListValue{ListValue: &pb.ValueList{Values: mapSlice(v.List(), encodeValue)}}}
	}
	return &pb.Value{}
}

func decodeValue(v *pb.Value) sdk.Value {
	switch x := v.GetValue().(type) {
	case *pb.Value_StringValue:
		return sdk.String(x.StringValue)
	case *pb.Value_NumberValue:
		return sdk.Number(x.NumberValue).In(sdk.Unit(v.GetUnit()))
	case *pb.Value_BoolValue:
		return sdk.Bool(x.BoolValue)
	case *pb.Value_TimeValue:
		return sdk.Time(time.Unix(0, x.TimeValue))
	case *pb.Value_ListValue:
		return sdk.List(mapSlice(x.ListValue.GetValues(), decodeValue)...)
	}
	return sdk.Value{}
}

func encodeAttrs(m map[string]sdk.Value) map[string]*pb.Value {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*pb.Value, len(m))
	for k, v := range m {
		out[k] = encodeValue(v)
	}
	return out
}

func decodeAttrs(m map[string]*pb.Value) map[string]sdk.Value {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]sdk.Value, len(m))
	for k, v := range m {
		out[k] = decodeValue(v)
	}
	return out
}

// encodeTime is t in Unix nanoseconds, or nil for the zero time.
func encodeTime(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	n := t.UnixNano()
	return &n
}

func decodeTime(n *int64) time.Time {
	if n == nil {
		return time.Time{}
	}
	return time.Unix(0, *n).UTC()
}

// mapSlice applies f to each element of in; an empty in gives nil.
func mapSlice[A, B any](in []A, f func(A) B) []B {
	if len(in) == 0 {
		return nil
	}
	out := make([]B, len(in))
	for i, a := range in {
		out[i] = f(a)
	}
	return out
}

// convertStrings converts between string types; an empty in gives nil.
func convertStrings[B, A ~string](in []A) []B {
	return mapSlice(in, func(a A) B { return B(a) })
}
