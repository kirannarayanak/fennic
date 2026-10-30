// Package interp runs a parsed Fennic program by walking its syntax tree.
//
// M1 is untyped: values carry their own kind at run time. Fennic values are
// Go values, so Go's garbage collector is Fennic's.
package interp

import (
	"fennic/ast"
)

// Value is any Fennic value:
//
//	int64, float64, string, bool   Int, Float, Str, Bool
//	*List                          List[T]
//	*Record                        a record, or an untyped record from json.parse
//	*Variant                       one variant of a sum type
//	noneValue                      the absent optional; a present optional is the bare value
//	*Failure                       the failure side of T!
//	unitValue                      what a function with no result returns
//	*Closure, *Func, *Bound, *Builtin  things that can be called
//	*TypeRef, *Module              names that `.` looks inside: `Config.default`, `fs.read`
type Value any

type noneValue struct{}
type unitValue struct{}

var (
	None Value = noneValue{}
	Unit Value = unitValue{}
)

type List struct{ Elems []Value }

// RecordType describes a declared record, or the untyped records json.parse
// builds (Name is empty and each record has its own field list).
type RecordType struct {
	Name   string
	Fields []string
}

func (t *RecordType) index(field string) int {
	for i, f := range t.Fields {
		if f == field {
			return i
		}
	}
	return -1
}

type Record struct {
	Type   *RecordType
	Fields []Value
}

type VariantType struct {
	Sum    string // the sum type's name
	Name   string
	Fields []string
}

type Variant struct {
	Type   *VariantType
	Fields []Value
}

// Failure is a failed result. Msg is what `"{err}"` shows. Fallback, when
// set, is source text for a sensible `or` default, used in the help for a
// failure nobody handled; it is cleared when the failure leaves a user
// function, whose result type M1 does not know.
type Failure struct {
	Msg       string
	Fallback  string
	cancelled bool // a task stopped because a sibling failed; never the reported cause
}

// Closure is a lambda with the scope it was created in.
type Closure struct {
	Lambda *ast.LambdaExpr
	Env    *Env
}

// Func is a declared function or method.
type Func struct {
	Decl *ast.FnDecl
}

// Bound is a method together with the value it was called on.
type Bound struct {
	Func *Func
	Self Value
}

type Builtin struct {
	Name string
	Fn   func(in *Interp, call *callSite, args []Value) Value
}

// TypeRef is a type name used as a value, as in `Config.default()`.
type TypeRef struct{ Name string }

type Module struct {
	Name    string
	Members map[string]Value
}

// kindName names a value's kind for error messages.
func kindName(v Value) string {
	switch v := v.(type) {
	case int64:
		return "Int"
	case float64:
		return "Float"
	case string:
		return "Str"
	case bool:
		return "Bool"
	case *List:
		return "List"
	case *Record:
		if v.Type.Name == "" {
			return "record"
		}
		return v.Type.Name
	case *Variant:
		return v.Type.Sum
	case noneValue:
		return "none"
	case *Failure:
		return "failure"
	case unitValue:
		return "nothing"
	case *Closure, *Func, *Bound, *Builtin:
		return "function"
	case *TypeRef:
		return "type " + v.Name
	case *Module:
		return "module " + v.Name
	}
	return "unknown"
}
