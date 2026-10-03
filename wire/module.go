package wire

import (
	"errors"

	"wayseer.dev/sdk"
	pb "wayseer.dev/sdk/proto/modulev1"

	"go.yaml.in/yaml/v3"
)

// EncodeInfo describes the module and the optional methods it implements, at Protocol.
func EncodeInfo(info sdk.Info, capabilities []string) *pb.InfoResponse {
	return &pb.InfoResponse{
		Module: EncodeVersion(Protocol), Kind: info.Kind, Version: info.Version,
		Description: info.Description, Capabilities: capabilities,
	}
}

// DecodeInfo returns the module's info, its optional methods, and the contract it speaks.
func DecodeInfo(r *pb.InfoResponse) (sdk.Info, []string, Version) {
	info := sdk.Info{Kind: r.GetKind(), Version: r.GetVersion(), Description: r.GetDescription()}
	return info, convertStrings[string](r.GetCapabilities()), DecodeVersion(r.GetModule())
}

// EncodeHealth sends an error as its message only.
func EncodeHealth(h sdk.Health) *pb.HealthResponse {
	r := &pb.HealthResponse{Disconnected: h.Disconnected, Note: h.Note}
	if h.Err != nil {
		msg := h.Err.Error()
		r.Error = &msg
	}
	return r
}

// DecodeHealth makes an error from the message.
func DecodeHealth(r *pb.HealthResponse) sdk.Health {
	h := sdk.Health{Disconnected: r.GetDisconnected(), Note: r.GetNote()}
	if r.Error != nil {
		h.Err = errors.New(*r.Error)
	}
	return h
}

// EncodeConfig keeps the options' lines, so errors name them as in process; comments are dropped.
func EncodeConfig(c sdk.Config) *pb.ConfigureRequest {
	r := &pb.ConfigureRequest{Name: string(c.Name), Line: int32(c.Line)}
	if c.Options.Kind != 0 {
		r.Options = encodeNode(&c.Options)
	}
	return r
}

// DecodeConfig converts a configure request from the contract.
func DecodeConfig(r *pb.ConfigureRequest) sdk.Config {
	c := sdk.Config{Name: sdk.ModuleID(r.GetName()), Line: int(r.GetLine())}
	if r.Options != nil {
		c.Options = *decodeNode(r.Options)
	}
	return c
}

// encodeNode encodes an alias with the node it refers to as its one child.
func encodeNode(n *yaml.Node) *pb.YamlNode {
	out := &pb.YamlNode{
		Kind: uint32(n.Kind), Style: uint32(n.Style), Tag: n.Tag, Value: n.Value,
		Anchor: n.Anchor, Line: int32(n.Line), Column: int32(n.Column),
		Content: mapSlice(n.Content, encodeNode),
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		out.Content = []*pb.YamlNode{encodeNode(n.Alias)}
	}
	return out
}

func decodeNode(n *pb.YamlNode) *yaml.Node {
	out := &yaml.Node{
		Kind: yaml.Kind(n.GetKind()), Style: yaml.Style(n.GetStyle()), Tag: n.GetTag(), Value: n.GetValue(),
		Anchor: n.GetAnchor(), Line: int(n.GetLine()), Column: int(n.GetColumn()),
		Content: mapSlice(n.GetContent(), decodeNode),
	}
	if out.Kind == yaml.AliasNode && len(out.Content) == 1 {
		out.Alias, out.Content = out.Content[0], nil
	}
	return out
}
