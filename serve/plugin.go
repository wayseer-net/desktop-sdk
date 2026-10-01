package serve

import (
	"context"
	pb "wayseer/pkg/sdk/proto/modulev1"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Handshake is shared by the app and every module; a process run without it exits with a hint.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "MINDSEYE_MODULE",
	MagicCookieValue: "d7c4a1f0-mindseye-tier2",
}

// PluginName is the one plugin a module process serves.
const PluginName = "module"

// Plugin connects a ModuleService server or client to go-plugin.
type Plugin struct {
	plugin.NetRPCUnsupportedPlugin
	Impl pb.ModuleServiceServer
}

// GRPCServer registers the module's service.
func (p *Plugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterModuleServiceServer(s, p.Impl)
	return nil
}

// GRPCClient returns a ModuleServiceClient over the process's connection.
func (p *Plugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return pb.NewModuleServiceClient(c), nil
}

// Plugins is the plugin set for serving s, or for a client when s is nil.
func Plugins(s pb.ModuleServiceServer) plugin.PluginSet {
	return plugin.PluginSet{PluginName: &Plugin{Impl: s}}
}
