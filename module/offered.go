package module

// ErrNotOffered is what a query answers when the module does not offer it, as an external
// module's host does for any query its process lacks. Callers skip that module.
var ErrNotOffered = notOffered("that")

// NotOffered is the error for a module that does not offer what, such as "event queries".
func NotOffered(what string) error { return notOffered(what) }

type notOffered string

func (n notOffered) Error() string { return "the module does not offer " + string(n) }

func (notOffered) Is(target error) bool { _, ok := target.(notOffered); return ok }
