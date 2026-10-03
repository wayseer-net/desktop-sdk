package wire

import (
	"fmt"
	"math"
	"time"
	"wayseer/pkg/sdk"
	"wayseer/pkg/sdk/data"
	pb "wayseer/pkg/sdk/proto/modulev1"
)

// EncodeMetrics converts a metric catalogue to the contract.
func EncodeMetrics(ms []sdk.Metric) *pb.MetricsResponse {
	return &pb.MetricsResponse{Metrics: mapSlice(ms, func(m sdk.Metric) *pb.Metric {
		return &pb.Metric{
			Name: m.Name, Unit: string(m.Unit), Description: m.Description,
			Kinds: convertStrings[string](m.Kinds), Native: m.Native, Extra: m.Extra, Joined: m.Joined,
		}
	})}
}

// DecodeMetrics converts a metric catalogue from the contract.
func DecodeMetrics(r *pb.MetricsResponse) []sdk.Metric {
	return mapSlice(r.GetMetrics(), func(m *pb.Metric) sdk.Metric {
		return sdk.Metric{
			Name: m.GetName(), Unit: sdk.Unit(m.GetUnit()), Description: m.GetDescription(),
			Kinds: convertStrings[sdk.Kind](m.GetKinds()), Native: m.GetNative(), Extra: m.GetExtra(), Joined: m.GetJoined(),
		}
	})
}

// EncodeSeriesQuery converts q; the filter's Offers and Linked stay with the host.
func EncodeSeriesQuery(q sdk.SeriesQuery) *pb.SeriesQuery {
	return &pb.SeriesQuery{
		Entities: convertStrings[string](q.Entities), Filter: encodeFilter(q.Filter), Metrics: q.Metrics,
		Window: encodeWindow(q.Window), Step: int64(q.Step), Agg: pb.Aggregation(q.Agg), Native: q.Native,
		Top: int32(min(max(q.Top, 0), math.MaxInt32)),
	}
}

// DecodeSeriesQuery converts a series query from the contract.
func DecodeSeriesQuery(q *pb.SeriesQuery) sdk.SeriesQuery {
	return sdk.SeriesQuery{
		Entities: convertStrings[sdk.EntityRef](q.GetEntities()), Filter: decodeFilter(q.GetFilter()),
		Metrics: convertStrings[string](q.GetMetrics()), Window: decodeWindow(q.GetWindow()),
		Step: time.Duration(q.GetStep()), Agg: sdk.Aggregation(q.GetAgg()), Native: q.GetNative(),
		Top: int(max(q.GetTop(), 0)),
	}
}

func encodeFilter(f data.Filter) *pb.Filter {
	return &pb.Filter{
		Kinds: convertStrings[string](f.Kinds), Sources: convertStrings[string](f.Sources),
		Statuses: mapSlice(f.Statuses, func(s sdk.StatusLevel) pb.StatusLevel { return pb.StatusLevel(s) }),
		Metrics:  f.Metrics, SameAs: convertStrings[string](f.SameAs), Tags: f.Tags,
		Attrs: mapSlice(f.Attrs, func(p data.Predicate) *pb.Predicate {
			return &pb.Predicate{Key: p.Key, Op: string(p.Op), Value: p.Value}
		}),
	}
}

func decodeFilter(f *pb.Filter) data.Filter {
	return data.Filter{
		Kinds: convertStrings[sdk.Kind](f.GetKinds()), Sources: convertStrings[sdk.ModuleID](f.GetSources()),
		Statuses: mapSlice(f.GetStatuses(), func(s pb.StatusLevel) sdk.StatusLevel { return sdk.StatusLevel(s) }),
		Metrics:  convertStrings[string](f.GetMetrics()), SameAs: convertStrings[sdk.EntityRef](f.GetSameAs()),
		Tags: convertStrings[string](f.GetTags()),
		Attrs: mapSlice(f.GetAttrs(), func(p *pb.Predicate) data.Predicate {
			return data.Predicate{Key: p.GetKey(), Op: data.Op(p.GetOp()), Value: p.GetValue()}
		}),
	}
}

func encodeWindow(w sdk.TimeWindow) *pb.TimeWindow {
	return &pb.TimeWindow{From: encodeTime(w.From), To: encodeTime(w.To)}
}

func decodeWindow(w *pb.TimeWindow) sdk.TimeWindow {
	if w == nil {
		return sdk.TimeWindow{}
	}
	return sdk.TimeWindow{From: decodeTime(w.From), To: decodeTime(w.To)}
}

// EncodeSeries converts series to the contract, with their points as two columns.
func EncodeSeries(ss []sdk.Series) []*pb.Series {
	return mapSlice(ss, func(s sdk.Series) *pb.Series {
		out := &pb.Series{Ref: &pb.SeriesRef{Entity: string(s.Ref.Entity), Metric: s.Ref.Metric}, Unit: string(s.Unit)}
		out.Times = mapSlice(s.Points, func(p sdk.Point) int64 { return p.T })
		out.Values = mapSlice(s.Points, func(p sdk.Point) float64 { return p.V })
		return out
	})
}

// DecodeSeries refuses a series whose columns differ in length.
func DecodeSeries(ss []*pb.Series) ([]sdk.Series, error) {
	out := make([]sdk.Series, 0, len(ss))
	for _, s := range ss {
		ts, vs := s.GetTimes(), s.GetValues()
		if len(ts) != len(vs) {
			return nil, fmt.Errorf("series %s %s: %d times and %d values", s.GetRef().GetEntity(), s.GetRef().GetMetric(), len(ts), len(vs))
		}
		ref := sdk.SeriesRef{Entity: sdk.EntityRef(s.GetRef().GetEntity()), Metric: s.GetRef().GetMetric()}
		var points []sdk.Point
		for i, t := range ts {
			points = append(points, sdk.Point{T: t, V: vs[i]})
		}
		out = append(out, sdk.Series{Ref: ref, Unit: sdk.Unit(s.GetUnit()), Points: points})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// EncodeEventQuery converts an event query to the contract.
func EncodeEventQuery(q sdk.EventQuery) *pb.EventQuery {
	return &pb.EventQuery{
		Entities: convertStrings[string](q.Entities), Window: encodeWindow(q.Window),
		MinSeverity: pb.Severity(q.MinSeverity), Kinds: q.Kinds, Limit: int32(q.Limit),
	}
}

// DecodeEventQuery converts an event query from the contract.
func DecodeEventQuery(q *pb.EventQuery) sdk.EventQuery {
	return sdk.EventQuery{
		Entities: convertStrings[sdk.EntityRef](q.GetEntities()), Window: decodeWindow(q.GetWindow()),
		MinSeverity: sdk.Severity(q.GetMinSeverity()), Kinds: convertStrings[string](q.GetKinds()), Limit: int(q.GetLimit()),
	}
}

// EncodeEvents converts events to the contract.
func EncodeEvents(evs []sdk.Event) []*pb.Event { return mapSlice(evs, encodeEvent) }

// DecodeEvents converts events from the contract.
func DecodeEvents(evs []*pb.Event) []sdk.Event { return mapSlice(evs, decodeEvent) }

// EncodeRefs converts search results to the contract.
func EncodeRefs(refs []sdk.EntityRef) []string { return convertStrings[string](refs) }

// DecodeRefs converts search results from the contract.
func DecodeRefs(refs []string) []sdk.EntityRef { return convertStrings[sdk.EntityRef](refs) }
