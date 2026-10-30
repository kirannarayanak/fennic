// Package ast defines the syntax tree. The M1 interpreter walks it, and the
// M2 type checker will check the same tree, so every node keeps the source
// span it came from.
package ast

import "github.com/kirannarayanak/fennic/token"

type Node interface {
	Pos() token.Pos // first character
	End() token.Pos // just past the last character
}

// Span is embedded in every node to give it Pos and End.
type Span struct{ From, To token.Pos }

func (s Span) Pos() token.Pos { return s.From }
func (s Span) End() token.Pos { return s.To }

// File is one source file: declarations and top-level statements in order.
type File struct {
	Span
	Items []Item
}

// Item is anything that can appear at the top level.
type Item interface {
	Node
	item()
}

// ---- Declarations ----

// FnDecl is `fn name(...)` or, with Recv set, `fn Type.name(...)`.
type FnDecl struct {
	Span
	Recv       *Ident // the type in `fn Console.write`; nil for a plain function
	Name       *Ident
	TypeParams []*TypeParam
	Params     []*Param
	Result     TypeExpr // nil when the function returns nothing
	Body       *Block
}

// HasSelf reports whether the first parameter is `self`.
func (f *FnDecl) HasSelf() bool {
	return len(f.Params) > 0 && f.Params[0].Name.Name == "self" && f.Params[0].Type == nil
}

type TypeParam struct {
	Span
	Name       *Ident
	Constraint TypeExpr // nil when unconstrained: `[T]`
}

// Param is `name: Type`, or `self` with a nil Type.
type Param struct {
	Span
	Name *Ident
	Type TypeExpr
}

// RecordDecl is `type User { name: Str ... }`.
type RecordDecl struct {
	Span
	Name   *Ident
	Fields []*FieldDecl
}

// SumDecl is `type Shape = | Circle(radius: Float) | ...`.
type SumDecl struct {
	Span
	Name     *Ident
	Variants []*Variant
}

type Variant struct {
	Span
	Name   *Ident
	Fields []*FieldDecl
}

type FieldDecl struct {
	Span
	Name *Ident
	Type TypeExpr
}

// InterfaceDecl is `interface Sink { fn write(self, line: Str) }`.
type InterfaceDecl struct {
	Span
	Name    *Ident
	Methods []*MethodSig
}

type MethodSig struct {
	Span
	Name   *Ident
	Params []*Param
	Result TypeExpr
}

// TestDecl is `test "name" { ... }`.
type TestDecl struct {
	Span
	Name string
	Body *Block
}

// ---- Types ----

type TypeExpr interface {
	Node
	typeExpr()
}

// NamedType is `Str`, `List[T]`.
type NamedType struct {
	Span
	Name *Ident
	Args []TypeExpr
}

// OptionalType is `T?`.
type OptionalType struct {
	Span
	Elem TypeExpr
}

// FailableType is `T!`.
type FailableType struct {
	Span
	Elem TypeExpr
}

// ---- Statements ----

type Stmt interface {
	Item
	stmt()
}

// LetStmt is `let x = e`, or `var x = e` with Mutable set.
type LetStmt struct {
	Span
	Mutable bool
	Name    *Ident
	Value   Expr
}

// AssignStmt is `x = e`.
// AssignStmt is `x = e`, or `x += e` with Op set to token.PlusEq.
type AssignStmt struct {
	Span
	Name  *Ident
	Op    token.Kind
	Value Expr
}

// ForStmt is `for x in items { ... }`. Var is `_` when the value is unused.
type ForStmt struct {
	Span
	Var  *Ident
	In   Expr
	Body *Block
}

type AssertStmt struct {
	Span
	Cond Expr
}

type ExprStmt struct {
	Span
	X Expr
}

// ---- Expressions ----

type Expr interface {
	Node
	expr()
}

type Ident struct {
	Span
	Name string
}

type IntLit struct {
	Span
	Value int64
}

type FloatLit struct {
	Span
	Value float64
}

type StrLit struct {
	Span
	Value string
}

// InterpStr is a string with `{expr}` parts. Parts alternate text and
// expressions; text parts are *StrLit, and empty text parts are kept so the
// order is always text, expr, text, ..., text.
type InterpStr struct {
	Span
	Parts []Expr
}

type BoolLit struct {
	Span
	Value bool
}

// NoneLit is `none`, the absent optional.
type NoneLit struct{ Span }

type ListLit struct {
	Span
	Elems []Expr
}

// RecordLit is `User { name: "Ada", age: 41 }`.
type RecordLit struct {
	Span
	Type   *Ident
	Fields []*FieldInit
}

type FieldInit struct {
	Span
	Name  *Ident
	Value Expr
}

// UnaryExpr is `-x`.
type UnaryExpr struct {
	Span
	Op token.Kind
	X  Expr
}

// BinaryExpr covers arithmetic, comparison and `??`.
type BinaryExpr struct {
	Span
	Op    token.Kind
	Left  Expr
	Right Expr
}

// PipeExpr is `x |> f(a)`, which means `f(x, a)`.
type PipeExpr struct {
	Span
	X    Expr
	Call *CallExpr
}

// CallExpr is `f(a)`, or `f[T](a)` with explicit type arguments.
type CallExpr struct {
	Span
	Fn       Expr
	TypeArgs []TypeExpr
	Args     []Expr
}

// IndexExpr is `xs[i]`.
type IndexExpr struct {
	Span
	X     Expr
	Index Expr
}

// TupleLit is `(a, b)`.
type TupleLit struct {
	Span
	Elems []Expr
}

// IfExpr is `if cond { ... } else { ... }`. Else is nil when there is no
// else branch, a *Block, or an *IfExpr for `else if`.
type IfExpr struct {
	Span
	Cond Expr
	Then *Block
	Else Expr
}

// FailExpr is `fail "message"`.
type FailExpr struct {
	Span
	Msg Expr
}

// FuncLit is an anonymous function: `fn() { ... }`.
type FuncLit struct {
	Span
	Params []*Param
	Result TypeExpr
	Body   *Block
}

// FieldExpr is `x.name`, or `x?.name` with Optional set.
type FieldExpr struct {
	Span
	X        Expr
	Name     *Ident
	Optional bool
}

// PropagateExpr is `x!`: on failure, return the failure from the enclosing
// function.
type PropagateExpr struct {
	Span
	X Expr
}

// ElseExpr is `x else fallback`, or `x else err -> handler` with ErrName
// set: the handling of a failure.
type ElseExpr struct {
	Span
	X        Expr
	ErrName  *Ident
	Fallback Expr
}

// WithExpr is `x with { age: 32 }`.
type WithExpr struct {
	Span
	X      Expr
	Fields []*FieldInit
}

// LambdaExpr is `x -> body`.
type LambdaExpr struct {
	Span
	Params []*Ident
	Body   Expr
}

// Block is `{ stmts }`. Its value is the last statement if that is an
// expression statement.
type Block struct {
	Span
	Stmts []Stmt
}

type MatchExpr struct {
	Span
	X    Expr
	Arms []*MatchArm
}

type MatchArm struct {
	Span
	Pattern Pattern
	Body    Expr
}

// Pattern is what a match arm compares a value with.
type Pattern interface {
	Node
	pattern()
}

// VariantPattern is `Circle(r)`, `Done(0)`, or `Queued` with no Args.
type VariantPattern struct {
	Span
	Name *Ident
	Args []Pattern
}

// BindPattern is a name, which matches anything and binds it.
type BindPattern struct {
	Span
	Name *Ident
}

// WildcardPattern is `_`, which matches anything and binds nothing.
type WildcardPattern struct{ Span }

// LiteralPattern is a literal such as `0`, `-1`, `"a"` or `true`.
type LiteralPattern struct {
	Span
	Value Expr // *IntLit, *FloatLit, *StrLit, *BoolLit, or *UnaryExpr of a number
}

// TuplePattern is `(p, q)`.
type TuplePattern struct {
	Span
	Elems []Pattern
}

// ListPattern is `[p, q]`, which matches a list of exactly that length.
type ListPattern struct {
	Span
	Elems []Pattern
}

// PatternBinds lists the names a pattern binds, in order.
func PatternBinds(p Pattern) []*Ident {
	switch p := p.(type) {
	case *BindPattern:
		return []*Ident{p.Name}
	case *VariantPattern:
		return bindsOf(p.Args)
	case *TuplePattern:
		return bindsOf(p.Elems)
	case *ListPattern:
		return bindsOf(p.Elems)
	}
	return nil
}

func bindsOf(ps []Pattern) []*Ident {
	var out []*Ident
	for _, p := range ps {
		out = append(out, PatternBinds(p)...)
	}
	return out
}

func (*VariantPattern) pattern()  {}
func (*BindPattern) pattern()     {}
func (*WildcardPattern) pattern() {}
func (*LiteralPattern) pattern()  {}
func (*TuplePattern) pattern()    {}
func (*ListPattern) pattern()     {}

// TogetherExpr is `together { ... }`.
type TogetherExpr struct {
	Span
	Body *Block
}

// BadExpr stands in for an expression that failed to parse, so the tree is
// complete even when the source is not.
type BadExpr struct{ Span }

// BadStmt stands in for a statement or declaration that failed to parse.
type BadStmt struct{ Span }

func (*FnDecl) item()        {}
func (*RecordDecl) item()    {}
func (*SumDecl) item()       {}
func (*InterfaceDecl) item() {}
func (*TestDecl) item()      {}
func (*LetStmt) item()       {}
func (*AssignStmt) item()    {}
func (*AssertStmt) item()    {}
func (*ExprStmt) item()      {}
func (*ForStmt) item()       {}
func (*BadStmt) item()       {}

func (*LetStmt) stmt()    {}
func (*AssignStmt) stmt() {}
func (*AssertStmt) stmt() {}
func (*ExprStmt) stmt()   {}
func (*ForStmt) stmt()    {}
func (*BadStmt) stmt()    {}

func (*NamedType) typeExpr()    {}
func (*OptionalType) typeExpr() {}
func (*FailableType) typeExpr() {}

func (*Ident) expr()         {}
func (*IntLit) expr()        {}
func (*FloatLit) expr()      {}
func (*StrLit) expr()        {}
func (*InterpStr) expr()     {}
func (*BoolLit) expr()       {}
func (*NoneLit) expr()       {}
func (*ListLit) expr()       {}
func (*RecordLit) expr()     {}
func (*UnaryExpr) expr()     {}
func (*BinaryExpr) expr()    {}
func (*PipeExpr) expr()      {}
func (*CallExpr) expr()      {}
func (*FieldExpr) expr()     {}
func (*PropagateExpr) expr() {}
func (*ElseExpr) expr()      {}
func (*IndexExpr) expr()     {}
func (*TupleLit) expr()      {}
func (*IfExpr) expr()        {}
func (*FailExpr) expr()      {}
func (*FuncLit) expr()       {}
func (*WithExpr) expr()      {}
func (*LambdaExpr) expr()    {}
func (*Block) expr()         {}
func (*MatchExpr) expr()     {}
func (*TogetherExpr) expr()  {}
func (*BadExpr) expr()       {}
