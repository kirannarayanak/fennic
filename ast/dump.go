package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// Dump renders a node as a one-line S-expression. Types print in source
// syntax (`List[T]?`). It exists for tests and for reading the tree.
func Dump(n Node) string {
	var b strings.Builder
	dump(&b, n)
	return b.String()
}

func dump(b *strings.Builder, n Node) {
	w := func(parts ...any) {
		for i, p := range parts {
			if i > 0 {
				b.WriteByte(' ')
			}
			switch p := p.(type) {
			case Node:
				dump(b, p)
			case string:
				b.WriteString(p)
			default:
				fmt.Fprint(b, p)
			}
		}
	}
	list := func(head string, rest ...any) {
		b.WriteByte('(')
		w(append([]any{head}, rest...)...)
		b.WriteByte(')')
	}

	switch n := n.(type) {
	case nil:
		b.WriteString("<nil>")
	case *File:
		args := make([]any, len(n.Items))
		for i, it := range n.Items {
			args[i] = it
		}
		list("file", args...)

	case *FnDecl:
		name := n.Name.Name
		if n.Recv != nil {
			name = n.Recv.Name + "." + name
		}
		args := []any{name}
		if len(n.TypeParams) > 0 {
			var tps []string
			for _, tp := range n.TypeParams {
				s := tp.Name.Name
				if tp.Constraint != nil {
					s += ": " + typeString(tp.Constraint)
				}
				tps = append(tps, s)
			}
			args = append(args, "["+strings.Join(tps, ", ")+"]")
		}
		args = append(args, paramsString(n.Params))
		if n.Result != nil {
			args = append(args, "->", typeString(n.Result))
		}
		args = append(args, n.Body)
		list("fn", args...)
	case *RecordDecl:
		args := []any{n.Name.Name}
		for _, f := range n.Fields {
			args = append(args, "("+f.Name.Name+" "+typeString(f.Type)+")")
		}
		list("type", args...)
	case *SumDecl:
		args := []any{n.Name.Name}
		for _, v := range n.Variants {
			s := []string{v.Name.Name}
			for _, f := range v.Fields {
				s = append(s, "("+f.Name.Name+" "+typeString(f.Type)+")")
			}
			args = append(args, "("+strings.Join(s, " ")+")")
		}
		list("sum", args...)
	case *InterfaceDecl:
		args := []any{n.Name.Name}
		for _, m := range n.Methods {
			s := "(fn " + m.Name.Name + " " + paramsString(m.Params)
			if m.Result != nil {
				s += " -> " + typeString(m.Result)
			}
			args = append(args, s+")")
		}
		list("interface", args...)
	case *TestDecl:
		list("test", strconv.Quote(n.Name), n.Body)

	case *LetStmt:
		head := "let"
		if n.Mutable {
			head = "var"
		}
		list(head, n.Name.Name, n.Value)
	case *AssignStmt:
		op := "="
		if n.Op != 0 {
			op = n.Op.String()
		}
		list(op, n.Name.Name, n.Value)
	case *IndexAssignStmt:
		list("=", "("+"index "+n.Name.Name+" "+Dump(n.Index)+")", n.Value)
	case *ForStmt:
		list("for", n.Var.Name, n.In, n.Body)
	case *AssertStmt:
		list("assert", n.Cond)
	case *ExprStmt:
		dump(b, n.X)
	case *BadStmt:
		b.WriteString("(bad)")

	case *Ident:
		b.WriteString(n.Name)
	case *IntLit:
		b.WriteString(strconv.FormatInt(n.Value, 10))
	case *FloatLit:
		s := strconv.FormatFloat(n.Value, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		b.WriteString(s)
	case *StrLit:
		b.WriteString(strconv.Quote(n.Value))
	case *InterpStr:
		args := make([]any, len(n.Parts))
		for i, p := range n.Parts {
			args[i] = p
		}
		list("str", args...)
	case *BoolLit:
		b.WriteString(strconv.FormatBool(n.Value))
	case *NoneLit:
		b.WriteString("none")
	case *ListLit:
		args := make([]any, len(n.Elems))
		for i, e := range n.Elems {
			args[i] = e
		}
		list("list", args...)
	case *RecordLit:
		args := []any{n.Type.Name}
		for _, f := range n.Fields {
			args = append(args, f)
		}
		list("record", args...)
	case *FieldInit:
		list(n.Name.Name, n.Value)
	case *UnaryExpr:
		list(n.Op.String(), n.X)
	case *BinaryExpr:
		list(n.Op.String(), n.Left, n.Right)
	case *PipeExpr:
		list("|>", n.X, n.Call)
	case *CallExpr:
		fn := any(n.Fn)
		if len(n.TypeArgs) > 0 {
			var ts []string
			for _, t := range n.TypeArgs {
				ts = append(ts, typeString(t))
			}
			fn = Dump(n.Fn) + "[" + strings.Join(ts, ", ") + "]"
		}
		args := []any{fn}
		for _, a := range n.Args {
			args = append(args, a)
		}
		list("call", args...)
	case *FieldExpr:
		op := "."
		if n.Optional {
			op = "?."
		}
		list(op, n.X, n.Name.Name)
	case *PropagateExpr:
		list("!", n.X)
	case *ElseExpr:
		if n.ErrName != nil {
			list("else", n.X, "("+n.ErrName.Name+")", n.Fallback)
		} else {
			list("else", n.X, n.Fallback)
		}
	case *IndexExpr:
		list("index", n.X, n.Index)
	case *MapLit:
		args := make([]any, len(n.Entries))
		for i, e := range n.Entries {
			args[i] = "(" + Dump(e.Key) + " " + Dump(e.Value) + ")"
		}
		list("map", args...)
	case *TupleLit:
		args := make([]any, len(n.Elems))
		for i, e := range n.Elems {
			args[i] = e
		}
		list("tuple", args...)
	case *IfExpr:
		if n.Else == nil {
			list("if", n.Cond, n.Then)
		} else {
			list("if", n.Cond, n.Then, n.Else)
		}
	case *FailExpr:
		list("fail", n.Msg)
	case *FuncLit:
		args := []any{paramsString(n.Params)}
		if n.Result != nil {
			args = append(args, "->", typeString(n.Result))
		}
		list("fn", append(args, n.Body)...)
	case *WithExpr:
		args := []any{n.X}
		for _, f := range n.Fields {
			args = append(args, f)
		}
		list("with", args...)
	case *LambdaExpr:
		var ps []string
		for _, p := range n.Params {
			ps = append(ps, p.Name)
		}
		list("lambda", "("+strings.Join(ps, " ")+")", n.Body)
	case *Block:
		args := make([]any, len(n.Stmts))
		for i, s := range n.Stmts {
			args[i] = s
		}
		list("block", args...)
	case *MatchExpr:
		args := []any{n.X}
		for _, a := range n.Arms {
			args = append(args, a)
		}
		list("match", args...)
	case *MatchArm:
		list("arm", n.Pattern, n.Body)
	case *VariantPattern:
		args := []any{n.Name.Name}
		for _, p := range n.Args {
			args = append(args, p)
		}
		list("pattern", args...)
	case *BindPattern:
		b.WriteString(n.Name.Name)
	case *WildcardPattern:
		b.WriteString("_")
	case *LiteralPattern:
		dump(b, n.Value)
	case *TuplePattern:
		args := make([]any, len(n.Elems))
		for i, p := range n.Elems {
			args[i] = p
		}
		list("tuple", args...)
	case *ListPattern:
		args := make([]any, len(n.Elems))
		for i, p := range n.Elems {
			args[i] = p
		}
		list("list", args...)
	case *TogetherExpr:
		list("together", n.Body)
	case *BadExpr:
		b.WriteString("(bad)")
	default:
		fmt.Fprintf(b, "<%T>", n)
	}
}

func paramsString(ps []*Param) string {
	var s []string
	for _, p := range ps {
		if p.Type == nil {
			s = append(s, p.Name.Name)
		} else {
			s = append(s, "("+p.Name.Name+" "+typeString(p.Type)+")")
		}
	}
	return "(" + strings.Join(s, " ") + ")"
}

func typeString(t TypeExpr) string {
	switch t := t.(type) {
	case *NamedType:
		if len(t.Args) == 0 {
			return t.Name.Name
		}
		var args []string
		for _, a := range t.Args {
			args = append(args, typeString(a))
		}
		return t.Name.Name + "[" + strings.Join(args, ", ") + "]"
	case *OptionalType:
		return typeString(t.Elem) + "?"
	case *FailableType:
		return typeString(t.Elem) + "!"
	case nil:
		return "<nil>"
	}
	return fmt.Sprintf("<%T>", t)
}
