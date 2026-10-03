package model

// mindsEyeRefs maps the internal module's entities as Mind's Eye named them to their Wayseer names.
var mindsEyeRefs = map[[2]string][2]string{
	{"mindseye/bus", ""}:              {"wayseer/bus", ""},
	{"mindseye/module", ""}:           {"wayseer/module", ""},
	{string(KindProcess), "mindseye"}: {string(KindProcess), "wayseer"},
}

// UpgradeRef returns the kind and native ID Wayseer gives an entity Mind's Eye saved by kind and
// native; any other entity is returned as given.
func UpgradeRef(kind Kind, native string) (Kind, string) {
	if to, ok := mindsEyeRefs[[2]string{string(kind), native}]; ok {
		return Kind(to[0]), to[1]
	}
	if to, ok := mindsEyeRefs[[2]string{string(kind), ""}]; ok {
		return Kind(to[0]), native
	}
	return kind, native
}
