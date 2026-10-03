package wire

import (
	"maps"

	"wayseer.dev/sdk"
	pb "wayseer.dev/sdk/proto/modulev1"
)

// EncodeActions converts a module's catalogue to the contract.
func EncodeActions(acts []sdk.Action) *pb.ActionsResponse {
	return &pb.ActionsResponse{Actions: mapSlice(acts, encodeAction)}
}

// DecodeActions converts a catalogue from the contract; the app validates it before use.
func DecodeActions(r *pb.ActionsResponse) []sdk.Action { return mapSlice(r.GetActions(), decodeAction) }

func encodeAction(a sdk.Action) *pb.Action {
	return &pb.Action{
		Id: a.ID, Title: a.Title, Changes: a.Changes,
		Kinds: convertStrings[string](a.Kinds), Params: mapSlice(a.Params, encodeParam),
	}
}

func decodeAction(a *pb.Action) sdk.Action {
	return sdk.Action{
		ID: a.GetId(), Title: a.GetTitle(), Changes: a.GetChanges(),
		Kinds: convertStrings[sdk.Kind](a.GetKinds()), Params: mapSlice(a.GetParams(), decodeParam),
	}
}

func encodeParam(p sdk.Param) *pb.Param {
	return &pb.Param{
		Name: p.Name, Title: p.Title, Type: pb.ParamType(p.Type),
		Min: p.Min, Max: p.Max, Choices: p.Choices, DefaultValue: p.Default,
	}
}

// decodeParam reads a type this build does not know as none, so validation refuses it.
func decodeParam(p *pb.Param) sdk.Param {
	var t sdk.ParamType
	if v := p.GetType(); v >= pb.ParamType_PARAM_TYPE_INT && v <= pb.ParamType_PARAM_TYPE_CHOICE {
		t = sdk.ParamType(v)
	}
	choices := p.GetChoices()
	if len(choices) == 0 {
		choices = nil
	}
	return sdk.Param{
		Name: p.GetName(), Title: p.GetTitle(), Type: t,
		Min: p.GetMin(), Max: p.GetMax(), Choices: choices, Default: p.GetDefaultValue(),
	}
}

// EncodeActionRequest converts a checked request to the contract.
func EncodeActionRequest(r sdk.ActionRequest) *pb.DoRequest {
	return &pb.DoRequest{Instance: string(r.Instance), Action: r.Action, Entity: string(r.Entity), Params: r.Params}
}

// DecodeActionRequest converts a request from the contract; no parameters are nil.
func DecodeActionRequest(r *pb.DoRequest) sdk.ActionRequest {
	var params map[string]string
	if len(r.GetParams()) > 0 {
		params = maps.Clone(r.GetParams())
	}
	return sdk.ActionRequest{
		Instance: sdk.ModuleID(r.GetInstance()), Action: r.GetAction(),
		Entity: sdk.EntityRef(r.GetEntity()), Params: params,
	}
}
