// Package types is the M2 type checker. It runs over the syntax tree after
// parsing and before anything runs. docs/types.md has the rules.
package types

import (
	"fmt"
	"strings"
)

// Type is one of the types below.
type Type interface{ typ() }

// Basic is Int, Float, Str, Bool, nothing (Unit) or the error bound by
// `or err ->`.
type Basic struct{ Name string }

var (
	Int   = &Basic{"Int"}
	Float = &Basic{"Float"}
	Str   = &Basic{"Str"}
	Bool  = &Basic{"Bool"}
	Unit  = &Basic{"nothing"}
	Error = &Basic{"Error"}
)

type List struct{ Elem Type }

// Opt is T?.
type Opt struct{ Elem Type }

// Fail is T!.
type Fail struct{ Elem Type }

// Named is a declared record or sum type. Records and sum types are nominal:
// two Named types are the same only if they are the same declaration.
type Named struct {
	Name     string
	Fields   []Field    // a record's fields, in declaration order
	Variants []*Variant // a sum type's variants; nil for a record
}

func (n *Named) IsSum() bool { return n.Variants != nil }

func (n *Named) field(name string) (Field, bool) {
	for _, f := range n.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

type Field struct {
	Name string
	Type Type
}

type Variant struct {
	Name   string
	Sum    *Named
	Fields []Field
}

// Iface is an interface: a set of method signatures.
type Iface struct {
	Name    string
	Methods []*Method
}

type Method struct {
	Name   string
	Params []Field // without self
	Result Type
}

// Param is a type parameter seen from inside its function's body. It is
// rigid: it stands for one unknown type and unifies only with itself.
type Param struct {
	Name       string
	Constraint Type // nil, Ord, or an *Iface
}

// Ord is the built-in constraint satisfied by Int, Float and Str.
var Ord = &Iface{Name: "Ord"}

// Func is the type of a lambda, a method value or a built-in's function
// argument. There is no syntax for it.
type Func struct {
	Params []Type
	Result Type
}

// Var is an unknown type that inference will decide.
type Var struct {
	id  int
	ref Type
}

// Bad is the type of something already reported as wrong. It unifies with
// everything, so one mistake gives one message.
type Bad struct{}

func (*Basic) typ() {}
func (*List) typ()  {}
func (*Opt) typ()   {}
func (*Fail) typ()  {}
func (*Named) typ() {}
func (*Iface) typ() {}
func (*Param) typ() {}
func (*Func) typ()  {}
func (*Var) typ()   {}
func (*Bad) typ()   {}

var bad = &Bad{}

// prune follows solved type variables to what they stand for.
func prune(t Type) Type {
	for {
		v, ok := t.(*Var)
		if !ok || v.ref == nil {
			return t
		}
		t = v.ref
	}
}

// Resolve replaces every solved variable inside t, so the result can be
// kept after checking ends.
func Resolve(t Type) Type {
	switch t := prune(t).(type) {
	case *List:
		return &List{Resolve(t.Elem)}
	case *Opt:
		return &Opt{Resolve(t.Elem)}
	case *Fail:
		return &Fail{Resolve(t.Elem)}
	case *Func:
		ps := make([]Type, len(t.Params))
		for i, p := range t.Params {
			ps[i] = Resolve(p)
		}
		return &Func{ps, Resolve(t.Result)}
	default:
		return t
	}
}

// String writes a type the way a program writes it.
func String(t Type) string {
	switch t := prune(t).(type) {
	case *Basic:
		return t.Name
	case *List:
		return "List[" + String(t.Elem) + "]"
	case *Opt:
		return String(t.Elem) + "?"
	case *Fail:
		return String(t.Elem) + "!"
	case *Named:
		return t.Name
	case *Iface:
		return t.Name
	case *Param:
		return t.Name
	case *Func:
		var ps []string
		for _, p := range t.Params {
			ps = append(ps, String(p))
		}
		return "(" + strings.Join(ps, ", ") + ") -> " + String(t.Result)
	case *Var:
		return "an unknown type"
	case *Bad:
		return "?"
	}
	return fmt.Sprintf("%T", t)
}

// article puts "a" or "an" before a type's name, as it is said aloud.
func article(t Type) string {
	s := String(t)
	if strings.HasPrefix(s, "an unknown type") {
		return s
	}
	return Article(s)
}

// Article puts "a" or "an" before a word: "an Int", "a User", "an Error".
func Article(s string) string {
	if s == "" || s == "nothing" {
		return s
	}
	vowel := func(b byte) bool { return strings.IndexByte("AEIOUaeiou", b) >= 0 }
	an := vowel(s[0])
	// "User", "Unit": a U said as "you" takes "a".
	if (s[0] == 'U' || s[0] == 'u') && len(s) > 2 && !vowel(s[1]) && vowel(s[2]) {
		an = false
	}
	if an {
		return "an " + s
	}
	return "a " + s
}

// plural names a type in the plural: "Ints", "Users".
func plural(t Type) string {
	switch prune(t).(type) {
	case *Basic, *Named, *Iface:
		return String(t) + "s"
	}
	return "`" + String(t) + "` values"
}

// ordered reports whether values of t can be compared with < and sorted.
func ordered(t Type) bool {
	switch t := prune(t).(type) {
	case *Basic:
		return t == Int || t == Float || t == Str
	case *Param:
		return t.Constraint == Ord
	case *Bad:
		return true
	}
	return false
}

func occurs(v *Var, t Type) bool {
	switch t := prune(t).(type) {
	case *Var:
		return t == v
	case *List:
		return occurs(v, t.Elem)
	case *Opt:
		return occurs(v, t.Elem)
	case *Fail:
		return occurs(v, t.Elem)
	case *Func:
		for _, p := range t.Params {
			if occurs(v, p) {
				return true
			}
		}
		return occurs(v, t.Result)
	}
	return false
}

// unify makes a and b the same type, solving variables as needed. It
// reports whether that was possible.
func unify(a, b Type) bool {
	a, b = prune(a), prune(b)
	if a == b {
		return true
	}
	if _, ok := a.(*Bad); ok {
		return true
	}
	if _, ok := b.(*Bad); ok {
		return true
	}
	if v, ok := a.(*Var); ok {
		if occurs(v, b) {
			return false
		}
		v.ref = b
		return true
	}
	if v, ok := b.(*Var); ok {
		if occurs(v, a) {
			return false
		}
		v.ref = a
		return true
	}
	switch x := a.(type) {
	case *Basic:
		y, ok := b.(*Basic)
		return ok && x.Name == y.Name
	case *List:
		y, ok := b.(*List)
		return ok && unify(x.Elem, y.Elem)
	case *Opt:
		y, ok := b.(*Opt)
		return ok && unify(x.Elem, y.Elem)
	case *Fail:
		y, ok := b.(*Fail)
		return ok && unify(x.Elem, y.Elem)
	case *Func:
		y, ok := b.(*Func)
		if !ok || len(x.Params) != len(y.Params) {
			return false
		}
		for i := range x.Params {
			if !unify(x.Params[i], y.Params[i]) {
				return false
			}
		}
		return unify(x.Result, y.Result)
	}
	// Named, Iface and Param are equal only to themselves, checked above.
	return false
}

// optional makes t optional, without making an optional doubly so.
func optional(t Type) Type {
	switch prune(t).(type) {
	case *Opt, *Bad:
		return t
	}
	return &Opt{t}
}

// failable makes t failable, without making a failable doubly so.
func failable(t Type) Type {
	switch prune(t).(type) {
	case *Fail, *Bad:
		return t
	}
	return &Fail{t}
}

// success is what t gives when it does not fail.
func success(t Type) Type {
	if f, ok := prune(t).(*Fail); ok {
		return f.Elem
	}
	return t
}
