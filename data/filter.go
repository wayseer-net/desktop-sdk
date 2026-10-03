package data

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"wayseer.dev/sdk/model"
	"wayseer.dev/sdk/units"
)

// Filter selects entities: any of Kinds (if set), from any of Sources (if set), a status at or
// worse than any of Statuses (if set), offering any of Metrics (if set), the same as any of
// SameAs (if set), all of Tags, and all of Attrs.
type Filter struct {
	Kinds    []model.Kind
	Sources  []model.ModuleID
	Statuses []model.StatusLevel
	Metrics  []string
	SameAs   []model.EntityRef
	Tags     []string
	Attrs    []Predicate
	Offers   Offers // what says which entities offer Metrics; nil offers none
	Linked   Linked // what says which entities are the same as another; nil links none
}

// Offers reports whether e has metric, as its module's catalogue says; entities do not know.
type Offers func(e *model.Entity, metric string) bool

// Linked reports whether e is joined to ref by same_as edges, as the world says.
type Linked func(e *model.Entity, ref model.EntityRef) bool

// IsZero reports whether f selects everything.
func (f Filter) IsZero() bool {
	return len(f.Kinds)+len(f.Sources)+len(f.Statuses)+len(f.Metrics)+len(f.SameAs)+len(f.Tags)+len(f.Attrs) == 0
}

// FilterError is a term that does not parse: where it starts (1-based byte column) and its length.
type FilterError struct {
	Col, Len int
	Err      error
}

func (e *FilterError) Error() string { return fmt.Sprintf("column %d: %v", e.Col, e.Err) }

func (e *FilterError) Unwrap() error { return e.Err }

// Predicate tests one attribute; the key "name" reads the entity's Name.
type Predicate struct {
	Key   string
	Op    Op
	Value string
}

// Op is a predicate comparison.
type Op string

// Operators, longest first so parsing prefers >= over >.
const (
	OpNE       Op = "!="
	OpGE       Op = ">="
	OpLE       Op = "<="
	OpEq       Op = "="
	OpGT       Op = ">"
	OpLT       Op = "<"
	OpContains Op = "~" // case-insensitive substring
	OpHas      Op = "has"
)

var infixOps = []Op{OpNE, OpGE, OpLE, OpEq, OpGT, OpLT, OpContains}

// ParseFilter reads the palette syntax: space-separated terms `kind:a,b`, `source:a,b`,
// `status:warn,crit`, `metric:a,b`, `same_as:<ref>`, `#tag`, `has:key`, and `key<op>value` with op one of = != < <= > >= ~. Values may be
// Go-quoted. Errors are *FilterError.
func ParseFilter(src string) (Filter, error) {
	var f Filter
	for col, term := range terms(src) {
		if err := f.addTerm(term); err != nil {
			return Filter{}, &FilterError{Col: col, Len: len(term), Err: err}
		}
	}
	return f, nil
}

// terms yields each whitespace-separated term with its 1-based byte column; quotes may hold spaces.
func terms(src string) func(func(int, string) bool) {
	return func(yield func(int, string) bool) {
		i := 0
		for i < len(src) {
			r, n := utf8.DecodeRuneInString(src[i:])
			if unicode.IsSpace(r) {
				i += n
				continue
			}
			start := i
			for i < len(src) {
				r, n := utf8.DecodeRuneInString(src[i:])
				if unicode.IsSpace(r) {
					break
				}
				if r == '"' {
					if q, err := strconv.QuotedPrefix(src[i:]); err == nil {
						i += len(q)
						continue
					}
				}
				i += n
			}
			if !yield(start+1, src[start:i]) {
				return
			}
		}
	}
}

func (f *Filter) addTerm(t string) error {
	switch {
	case strings.HasPrefix(t, "kind:"):
		return f.addKinds(t[len("kind:"):])
	case strings.HasPrefix(t, "source:"):
		return f.addSources(t[len("source:"):])
	case strings.HasPrefix(t, "status:"):
		return f.addStatuses(t[len("status:"):])
	case strings.HasPrefix(t, "metric:"):
		return f.addMetrics(t[len("metric:"):])
	case strings.HasPrefix(t, "same_as:"):
		return f.addSameAs(t[len("same_as:"):])
	case strings.HasPrefix(t, "has:"):
		k := t[len("has:"):]
		if !validKey(k) {
			return fmt.Errorf("has: needs an attribute name, got %q", k)
		}
		f.Attrs = append(f.Attrs, Predicate{Key: k, Op: OpHas})
		return nil
	case strings.HasPrefix(t, "#"):
		tag, err := value(t[1:])
		if err != nil || tag == "" {
			return fmt.Errorf("# needs a tag")
		}
		f.Tags = append(f.Tags, tag)
		return nil
	}
	return f.addPredicate(t)
}

func (f *Filter) addKinds(list string) error {
	for k := range strings.SplitSeq(list, ",") {
		kind := model.Kind(k)
		if err := kind.Validate(); err != nil {
			return err
		}
		f.Kinds = append(f.Kinds, kind)
	}
	return nil
}

func (f *Filter) addSources(list string) error {
	for id := range strings.SplitSeq(list, ",") {
		src := model.ModuleID(id)
		if err := src.Validate(); err != nil {
			return err
		}
		f.Sources = append(f.Sources, src)
	}
	return nil
}

func (f *Filter) addMetrics(list string) error {
	for name := range strings.SplitSeq(list, ",") {
		if !validKey(name) {
			return fmt.Errorf("metric: needs a metric name, such as cpu.utilisation, got %q", name)
		}
		f.Metrics = append(f.Metrics, name)
	}
	return nil
}

func (f *Filter) addSameAs(s string) error {
	v, err := value(s)
	if err != nil {
		return err
	}
	ref := model.EntityRef(v)
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("same_as: needs an entity ref, such as prom/host/db-07: %w", err)
	}
	f.SameAs = append(f.SameAs, ref)
	return nil
}

func (f *Filter) addStatuses(list string) error {
	for name := range strings.SplitSeq(list, ",") {
		l, ok := statusNamed(name)
		if !ok {
			return fmt.Errorf("status: needs one of unknown, ok, warn, crit, down, got %q", name)
		}
		f.Statuses = append(f.Statuses, l)
	}
	return nil
}

// statusNamed is the status level called name.
func statusNamed(name string) (model.StatusLevel, bool) {
	for l := model.StatusUnknown; l <= model.StatusDown; l++ {
		if l.String() == name {
			return l, true
		}
	}
	return 0, false
}

func (f *Filter) addPredicate(t string) error {
	at := strings.IndexAny(t, "!=<>~")
	if at <= 0 || !validKey(t[:at]) {
		return fmt.Errorf("%q is not kind:, #tag, has:key or key<op>value", t)
	}
	for _, op := range infixOps {
		if rest, ok := strings.CutPrefix(t[at:], string(op)); ok {
			v, err := value(rest)
			if err != nil {
				return err
			}
			f.Attrs = append(f.Attrs, Predicate{Key: t[:at], Op: op, Value: v})
			return nil
		}
	}
	return fmt.Errorf("%q: unknown operator", t)
}

// value unquotes s if it is quoted; a quoted value must be the whole term.
func value(s string) (string, error) {
	if !strings.HasPrefix(s, `"`) {
		return s, nil
	}
	q, err := strconv.QuotedPrefix(s)
	if err != nil || len(q) != len(s) {
		return "", fmt.Errorf("bad quoted value %s", s)
	}
	return strconv.Unquote(q)
}

// validKey allows letters, digits and _ . - / in attribute names.
func validKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_.-/", r) {
			return false
		}
	}
	return true
}

// String writes f in the syntax ParseFilter reads.
func (f Filter) String() string {
	var parts []string
	if len(f.Kinds) > 0 {
		ks := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			ks[i] = string(k)
		}
		parts = append(parts, "kind:"+strings.Join(ks, ","))
	}
	if len(f.Sources) > 0 {
		ss := make([]string, len(f.Sources))
		for i, s := range f.Sources {
			ss[i] = string(s)
		}
		parts = append(parts, "source:"+strings.Join(ss, ","))
	}
	if len(f.Statuses) > 0 {
		ls := make([]string, len(f.Statuses))
		for i, l := range f.Statuses {
			ls[i] = l.String()
		}
		parts = append(parts, "status:"+strings.Join(ls, ","))
	}
	if len(f.Metrics) > 0 {
		parts = append(parts, "metric:"+strings.Join(f.Metrics, ","))
	}
	for _, r := range f.SameAs {
		parts = append(parts, "same_as:"+quote(string(r)))
	}
	for _, t := range f.Tags {
		parts = append(parts, "#"+quote(t))
	}
	for _, p := range f.Attrs {
		if p.Op == OpHas {
			parts = append(parts, "has:"+p.Key)
		} else {
			parts = append(parts, p.Key+string(p.Op)+quote(p.Value))
		}
	}
	return strings.Join(parts, " ")
}

// quote leaves plain values bare and Go-quotes the rest, and those starting with an operator
// character, which would read as part of the operator.
func quote(s string) string {
	plain := s != "" && !strings.ContainsAny(s[:1], `"!=<>~`) && utf8.ValidString(s) &&
		!strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '"' || !unicode.IsPrint(r) })
	if plain {
		return s
	}
	return strconv.Quote(s)
}

// Match reports whether e satisfies every part of f.
func (f Filter) Match(e *model.Entity) bool {
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, e.Kind) {
		return false
	}
	if len(f.Sources) > 0 && !slices.Contains(f.Sources, e.Source) {
		return false
	}
	if len(f.Statuses) > 0 && e.Status.Level < slices.Min(f.Statuses) {
		return false
	}
	if len(f.Metrics) > 0 && !f.offersAny(e) {
		return false
	}
	if len(f.SameAs) > 0 && !f.sameAsAny(e) {
		return false
	}
	for _, t := range f.Tags {
		if !slices.Contains(e.Tags, t) {
			return false
		}
	}
	for _, p := range f.Attrs {
		if !p.Match(e) {
			return false
		}
	}
	return true
}

func (f Filter) offersAny(e *model.Entity) bool {
	return f.Offers != nil && slices.ContainsFunc(f.Metrics, func(m string) bool { return f.Offers(e, m) })
}

func (f Filter) sameAsAny(e *model.Entity) bool {
	return slices.ContainsFunc(f.SameAs, func(r model.EntityRef) bool {
		return e.Ref == r || f.Linked != nil && f.Linked(e, r)
	})
}

// Match tests p against e; a list attribute matches if any element does, except for !=.
func (p Predicate) Match(e *model.Entity) bool {
	v, ok := e.Attrs[p.Key]
	if p.Key == "name" && !ok {
		v, ok = model.String(e.Name), true
	}
	switch {
	case p.Op == OpHas:
		return ok
	case !ok:
		return p.Op == OpNE
	case v.Type() == model.TypeList && p.Op == OpNE:
		return !slices.ContainsFunc(v.List(), Predicate{p.Key, OpEq, p.Value}.test)
	case v.Type() == model.TypeList:
		return slices.ContainsFunc(v.List(), p.test)
	}
	return p.test(v)
}

// test compares one scalar value, numerically when v is a number and p's value a quantity in
// its unit, such as 8GiB.
func (p Predicate) test(v model.Value) bool {
	if p.Op == OpContains {
		return strings.Contains(strings.ToLower(v.String()), strings.ToLower(p.Value))
	}
	var c int
	if want, ok := units.Parse(p.Value, v.Unit()); ok && v.Type() == model.TypeNumber {
		if math.IsNaN(v.Num()) || math.IsNaN(want) {
			return p.Op == OpNE // NaN is unordered and equal to nothing
		}
		c = cmp.Compare(v.Num(), want)
	} else {
		c = strings.Compare(v.String(), p.Value)
	}
	switch p.Op {
	case OpEq:
		return c == 0
	case OpNE:
		return c != 0
	case OpLT:
		return c < 0
	case OpLE:
		return c <= 0
	case OpGT:
		return c > 0
	case OpGE:
		return c >= 0
	}
	return false
}
