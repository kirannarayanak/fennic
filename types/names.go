package types

import (
	"strings"

	"github.com/kirannarayanak/fennic/ast"
)

// Name is what one identifier in the program names, for the language
// server's hover and go-to-definition.
type Name struct {
	// Decl is the identifier that declares it: a let, var, parameter, for
	// variable, pattern binding, function, method, type, variant or field.
	// It is nil for something built in.
	Decl *ast.Ident
	// Type is its type, for a value.
	Type Type
	// Sig is how it is declared, for a function, method or type:
	// `fn greet(who: Str) -> Str`, or a built-in at the types it is used
	// with, `fn len(List[Int]) -> Int`. When it is set, it is what a hover
	// shows instead of the type.
	Sig string
}

// Hover is what the language server shows for the name at id.
func (n *Name) Hover(id *ast.Ident) string {
	if n.Sig != "" {
		return n.Sig
	}
	return id.Name + ": " + String(n.Type)
}

// decls are the declaring identifiers of the program's types, variants and
// fields, by name.
type decls struct {
	types    map[string]*ast.Ident
	variants map[string]*ast.Ident
	fields   map[string]map[string]*ast.Ident // record or variant name -> field
}

func (c *checker) declareNames(file *ast.File) {
	c.decl = decls{types: map[string]*ast.Ident{}, variants: map[string]*ast.Ident{}, fields: map[string]map[string]*ast.Ident{}}
	addFields := func(owner string, fs []*ast.FieldDecl) {
		m := map[string]*ast.Ident{}
		for _, f := range fs {
			m[f.Name.Name] = f.Name
		}
		c.decl.fields[owner] = m
	}
	for _, it := range file.Items {
		switch d := it.(type) {
		case *ast.RecordDecl:
			c.decl.types[d.Name.Name] = d.Name
			c.name(d.Name, &Name{Decl: d.Name, Sig: "type " + d.Name.Name})
			addFields(d.Name.Name, d.Fields)
		case *ast.SumDecl:
			c.decl.types[d.Name.Name] = d.Name
			c.name(d.Name, &Name{Decl: d.Name, Sig: "type " + d.Name.Name})
			for _, v := range d.Variants {
				c.decl.variants[v.Name.Name] = v.Name
				c.name(v.Name, &Name{Decl: v.Name, Sig: c.text(v)})
				addFields(v.Name.Name, v.Fields)
			}
		case *ast.InterfaceDecl:
			c.decl.types[d.Name.Name] = d.Name
			c.name(d.Name, &Name{Decl: d.Name, Sig: "interface " + d.Name.Name})
		case *ast.FnDecl:
			if d.Recv != nil {
				c.typeName(d.Recv)
			}
			c.name(d.Name, &Name{Decl: d.Name, Sig: c.fnSig(d)})
		}
	}
}

// name records what id names. The first record for an identifier stands.
func (c *checker) name(id *ast.Ident, n *Name) {
	if id == nil {
		return
	}
	if _, done := c.info.Names[id]; !done {
		c.info.Names[id] = n
	}
}

// typeName records a use of a type's name.
func (c *checker) typeName(id *ast.Ident) {
	if d, ok := c.decl.types[id.Name]; ok {
		c.name(id, c.info.Names[d])
	}
}

// field records a use of a field of the record or variant called owner.
func (c *checker) fieldName(owner string, id *ast.Ident, t Type) {
	c.name(id, &Name{Decl: c.decl.fields[owner][id.Name], Type: t})
}

// fnSig is a declared function's signature as the source writes it, up to
// its body.
func (c *checker) fnSig(d *ast.FnDecl) string {
	end := d.End().Offset
	if d.Body != nil {
		end = d.Body.Pos().Offset
	}
	return strings.Join(strings.Fields(c.src[d.Pos().Offset:end]), " ")
}

// sigName records a call of a declared function or method.
func (c *checker) sigName(id *ast.Ident, s *sig) {
	c.name(id, &Name{Decl: s.decl.Name, Sig: c.fnSig(s.decl)})
}

// builtinCall is a call of a built-in, whose signature is shown at the
// types of its arguments once they are known.
type builtinCall struct {
	id     *ast.Ident
	name   string
	args   []ast.Expr
	result Type
}

func (c *checker) finishNames() {
	for _, b := range c.builtinCalls {
		args := make([]string, len(b.args))
		for i, a := range b.args {
			args[i] = String(Resolve(c.types[a]))
		}
		sig := "fn " + b.name + "(" + strings.Join(args, ", ") + ")"
		if r := Resolve(b.result); r != Unit {
			sig += " -> " + String(r)
		}
		c.name(b.id, &Name{Sig: sig})
	}
	for owner, fields := range c.decl.fields {
		n, ok := c.named[owner]
		if !ok {
			continue // a variant's fields are named in patterns, not read with `.`
		}
		for _, f := range n.Fields {
			if id := fields[f.Name]; id != nil {
				c.name(id, &Name{Decl: id, Type: f.Type})
			}
		}
	}
	for _, n := range c.info.Names {
		if n.Type != nil {
			n.Type = Resolve(n.Type)
		}
	}
}
