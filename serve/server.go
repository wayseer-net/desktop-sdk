package serve

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"wayseer.dev/sdk"
	pb "wayseer.dev/sdk/proto/modulev1"
	"wayseer.dev/sdk/wire"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Main serves m to the app that started this process, and returns when the app is done with it.
func Main(m sdk.Module) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         Plugins(NewServer(m)),
		GRPCServer:      plugin.DefaultGRPCServer,
		Logger:          hclog.New(&hclog.LoggerOptions{Name: "serve", Level: hclog.Warn, Output: os.Stderr}),
	})
}

// NewServer answers the module contract with m; the app configures it before running it.
func NewServer(m sdk.Module) pb.ModuleServiceServer { return &server{m: m} }

type server struct {
	pb.UnimplementedModuleServiceServer
	m sdk.Module
}

func (s *server) Info(context.Context, *pb.InfoRequest) (*pb.InfoResponse, error) {
	return wire.EncodeInfo(s.m.Info(), sdk.Capabilities(s.m)), nil
}

func (s *server) Configure(ctx context.Context, r *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if err := guard(func() error { return s.m.Configure(ctx, wire.DecodeConfig(r)) }); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pb.ConfigureResponse{}, nil
}

func (s *server) Health(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error) {
	var h sdk.Health
	if err := guard(func() error { h = s.m.Health(); return nil }); err != nil {
		h = sdk.Health{Err: err}
	}
	return wire.EncodeHealth(h), nil
}

func (s *server) Run(_ *pb.RunRequest, st grpc.ServerStreamingServer[pb.RunResponse]) error {
	k := &streamSink{send: st.Send}
	return failed(guard(func() error { return s.m.Run(st.Context(), k) }))
}

func (s *server) Discover(ctx context.Context, _ *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	d, ok := s.m.(sdk.Discoverer)
	if !ok {
		return nil, unimplemented("discovery")
	}
	var cs *sdk.ChangeSet
	err := guard(func() (err error) { cs, err = d.Discover(ctx); return err })
	return &pb.DiscoverResponse{ChangeSet: wire.EncodeChangeSet(cs)}, failed(err)
}

func (s *server) Metrics(context.Context, *pb.MetricsRequest) (*pb.MetricsResponse, error) {
	q, ok := s.m.(sdk.SeriesQuerier)
	if !ok {
		return nil, unimplemented("series queries")
	}
	var ms []sdk.Metric
	err := guard(func() error { ms = q.Metrics(); return nil })
	return wire.EncodeMetrics(ms), failed(err)
}

func (s *server) QuerySeries(ctx context.Context, r *pb.QuerySeriesRequest) (*pb.QuerySeriesResponse, error) {
	q, ok := s.m.(sdk.SeriesQuerier)
	if !ok {
		return nil, unimplemented("series queries")
	}
	var ss []sdk.Series
	err := guard(func() (err error) { ss, err = q.QuerySeries(ctx, wire.DecodeSeriesQuery(r.GetQuery())); return err })
	return &pb.QuerySeriesResponse{Series: wire.EncodeSeries(ss)}, failed(err)
}

func (s *server) QueryEvents(ctx context.Context, r *pb.QueryEventsRequest) (*pb.QueryEventsResponse, error) {
	q, ok := s.m.(sdk.EventQuerier)
	if !ok {
		return nil, unimplemented("event queries")
	}
	var evs []sdk.Event
	err := guard(func() (err error) { evs, err = q.QueryEvents(ctx, wire.DecodeEventQuery(r.GetQuery())); return err })
	return &pb.QueryEventsResponse{Events: wire.EncodeEvents(evs)}, failed(err)
}

func (s *server) Subscribe(r *pb.SubscribeRequest, st grpc.ServerStreamingServer[pb.SubscribeResponse]) error {
	sub, ok := s.m.(sdk.Subscriber)
	if !ok {
		return unimplemented("subscriptions")
	}
	var mu sync.Mutex
	send := func(ss []sdk.Series) {
		mu.Lock()
		defer mu.Unlock()
		_ = st.Send(&pb.SubscribeResponse{Series: wire.EncodeSeries(ss)}) // a broken stream ends the context
	}
	q := wire.DecodeSeriesQuery(r.GetQuery())
	return failed(guard(func() error { return sub.Subscribe(st.Context(), q, send) }))
}

func (s *server) Search(ctx context.Context, r *pb.SearchRequest) (*pb.SearchResponse, error) {
	q, ok := s.m.(sdk.Searcher)
	if !ok {
		return nil, unimplemented("search")
	}
	var refs []sdk.EntityRef
	err := guard(func() (err error) { refs, err = q.Search(ctx, r.GetText(), int(r.GetLimit())); return err })
	return &pb.SearchResponse{Refs: wire.EncodeRefs(refs)}, failed(err)
}

func (s *server) Actions(context.Context, *pb.ActionsRequest) (*pb.ActionsResponse, error) {
	a, ok := s.m.(sdk.Actor)
	if !ok {
		return nil, unimplemented("actions")
	}
	var acts []sdk.Action
	err := guard(func() error { acts = a.Actions(); return nil })
	return wire.EncodeActions(acts), failed(err)
}

// Do runs the action until it ends or the app cancels the call.
func (s *server) Do(ctx context.Context, r *pb.DoRequest) (*pb.DoResponse, error) {
	a, ok := s.m.(sdk.Actor)
	if !ok {
		return nil, unimplemented("actions")
	}
	var res sdk.ActionResult
	err := guard(func() (err error) { res, err = a.Do(ctx, wire.DecodeActionRequest(r)); return err })
	return &pb.DoResponse{Message: res.Message}, failed(err)
}

// streamSink sends change sets down the Run stream in parts; Send is not safe for concurrent use.
type streamSink struct {
	mu   sync.Mutex
	send func(*pb.RunResponse) error
}

func (k *streamSink) Snapshot(ctx context.Context, cs *sdk.ChangeSet) error {
	return k.put(ctx, true, cs)
}

func (k *streamSink) Delta(ctx context.Context, cs *sdk.ChangeSet) error {
	return k.put(ctx, false, cs)
}

func (k *streamSink) put(ctx context.Context, snapshot bool, cs *sdk.ChangeSet) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, p := range wire.Parts(snapshot, wire.EncodeChangeSet(cs), wire.MaxPart) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := k.send(p); err != nil {
			return err
		}
	}
	return nil
}

func unimplemented(what string) error {
	return status.Errorf(codes.Unimplemented, "the module does not offer %s", what)
}

// failed carries err's message to the app.
func failed(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Unknown, err.Error())
}

// guard turns a panic into an error, printing its stack to stderr, which the app logs.
func guard(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n%s", r, debug.Stack())
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return f()
}
