package interp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"unicode/utf8"
)

// prelude is the scope above the program's globals: the built-in functions
// and modules. A program may shadow any of them.
func (in *Interp) prelude() *Env {
	env := NewEnv(nil)
	def := func(name string, fn func(in *Interp, call *callSite, args []Value) Value) {
		env.Define(name, &Builtin{Name: name, Fn: fn}, false)
	}
	def("print", builtinPrint)
	def("len", builtinLen)
	def("first", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "first", args, 1)
		if len(l.Elems) == 0 {
			return None
		}
		return l.Elems[0]
	})
	def("last", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "last", args, 1)
		if len(l.Elems) == 0 {
			return None
		}
		return l.Elems[len(l.Elems)-1]
	})
	def("sum", builtinSum)
	def("map", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "map", args, 2)
		out := make([]Value, len(l.Elems))
		for i, e := range l.Elems {
			out[i] = in.call(args[1], []Value{e}, c)
		}
		return &List{Elems: out}
	})
	def("each", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "each", args, 2)
		for _, e := range l.Elems {
			in.call(args[1], []Value{e}, c)
		}
		return Unit
	})
	def("filter", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "filter", args, 2)
		var out []Value
		for _, e := range l.Elems {
			keep, ok := in.call(args[1], []Value{e}, c).(bool)
			if !ok {
				in.fail(c.node, "give filter a function that returns a Bool, like `x -> x > 0`", "filter's function must return a Bool")
			}
			if keep {
				out = append(out, e)
			}
		}
		return &List{Elems: out}
	})
	def("sort_by", builtinSortBy)
	def("take", func(in *Interp, c *callSite, args []Value) Value {
		l := in.listArg(c, "take", args, 2)
		n, ok := args[1].(int64)
		if !ok || n < 0 {
			in.fail(c.node, "write a count, like `take(10)`", "take needs a count that is an Int of 0 or more")
		}
		if int(n) > len(l.Elems) {
			n = int64(len(l.Elems))
		}
		return &List{Elems: append([]Value(nil), l.Elems[:n]...)}
	})
	def("map_par", builtinMapPar)

	env.Define("fs", &Module{Name: "fs", Members: map[string]Value{
		"read": &Builtin{Name: "fs.read", Fn: builtinRead},
	}}, false)
	env.Define("json", &Module{Name: "json", Members: map[string]Value{
		"parse": &Builtin{Name: "json.parse", Fn: builtinParseJSON},
	}}, false)
	env.Define("log", &Module{Name: "log", Members: map[string]Value{
		"warn": &Builtin{Name: "log.warn", Fn: func(in *Interp, c *callSite, args []Value) Value {
			in.print(in.errw, "warn: "+in.strArg(c, "log.warn", args)+"\n")
			return Unit
		}},
	}}, false)
	return env
}

func (in *Interp) arity(c *callSite, name string, args []Value, n int) {
	if len(args) != n {
		in.fail(c.node, "", "`%s` takes %d %s, but was given %d", name, n, plural(n, "argument", "arguments"), len(args))
	}
}

func (in *Interp) listArg(c *callSite, name string, args []Value, n int) *List {
	in.arity(c, name, args, n)
	l, ok := args[0].(*List)
	if !ok {
		in.fail(c.node, "", "`%s` needs a List, but was given %s", name, article(kindName(args[0])))
	}
	return l
}

func (in *Interp) strArg(c *callSite, name string, args []Value) string {
	in.arity(c, name, args, 1)
	s, ok := args[0].(string)
	if !ok {
		in.fail(c.node, fmt.Sprintf(`turn it into a Str with interpolation: %s("{x}")`, name),
			"`%s` needs a Str, but was given %s", name, article(kindName(args[0])))
	}
	return s
}

func builtinPrint(in *Interp, c *callSite, args []Value) Value {
	in.print(in.out, in.strArg(c, "print", args)+"\n")
	return Unit
}

func builtinLen(in *Interp, c *callSite, args []Value) Value {
	in.arity(c, "len", args, 1)
	switch v := args[0].(type) {
	case *List:
		return int64(len(v.Elems))
	case string:
		return int64(utf8.RuneCountInString(v))
	}
	in.fail(c.node, "", "`len` needs a List or a Str, but was given %s", article(kindName(args[0])))
	return nil
}

func builtinSum(in *Interp, c *callSite, args []Value) Value {
	l := in.listArg(c, "sum", args, 1)
	if len(l.Elems) == 0 {
		return int64(0)
	}
	switch l.Elems[0].(type) {
	case int64:
		var total int64
		for _, e := range l.Elems {
			n, ok := e.(int64)
			if !ok {
				in.fail(c.node, "", "`sum` needs a list of all Ints or all Floats")
			}
			total += n
		}
		return total
	case float64:
		var total float64
		for _, e := range l.Elems {
			n, ok := e.(float64)
			if !ok {
				in.fail(c.node, "", "`sum` needs a list of all Ints or all Floats")
			}
			total += n
		}
		return total
	}
	in.fail(c.node, "", "`sum` needs a list of numbers, but it holds %s", article(kindName(l.Elems[0])))
	return nil
}

// builtinSortBy sorts by a key, keeping equal keys in their original order.
func builtinSortBy(in *Interp, c *callSite, args []Value) Value {
	l := in.listArg(c, "sort_by", args, 2)
	keys := make([]Value, len(l.Elems))
	for i, e := range l.Elems {
		keys[i] = in.call(args[1], []Value{e}, c)
	}
	idx := make([]int, len(l.Elems))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ka, kb := keys[idx[a]], keys[idx[b]]
		r, ok := compare(ka, kb)
		if !ok {
			if kindName(ka) != kindName(kb) {
				in.fail(c.node, "return the same kind of key for every element", "sort_by keys must all be one kind, but found %s and %s",
					article(kindName(ka)), article(kindName(kb)))
			}
			in.fail(c.node, "return an Int, a Float or a Str from the key function", "sort_by cannot order %s keys", kindName(ka))
		}
		return r < 0
	})
	out := make([]Value, len(idx))
	for i, j := range idx {
		out[i] = l.Elems[j]
	}
	return &List{Elems: out}
}

// builtinMapPar runs the function on every element as its own task inside
// the enclosing `together` scope. Results keep input order. The first task to
// fail cancels the scope, and its failure is the result: the tasks it
// cancelled are never reported. If several tasks fail on their own, which of
// them came first depends on timing.
func builtinMapPar(in *Interp, c *callSite, args []Value) Value {
	l := in.listArg(c, "map_par", args, 2)
	if !c.t.together {
		in.fail(c.node, "wrap it: `together { xs |> map_par(f) }!`", "map_par spawns tasks, so it must run inside a `together` block")
	}
	results := make([]Value, len(l.Elems))
	panics := make([]any, len(l.Elems))
	var (
		wg    sync.WaitGroup
		once  sync.Once
		cause *Failure
	)
	for i, e := range l.Elems {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics[i] = r
					c.t.cancel()
				}
			}()
			results[i] = in.call(args[1], []Value{e}, c)
			if f, failed := results[i].(*Failure); failed && !f.cancelled {
				once.Do(func() { cause = f })
				c.t.cancel()
			}
		}()
	}
	wg.Wait()
	for _, p := range panics {
		if p != nil {
			panic(p)
		}
	}
	if cause != nil {
		return cause
	}
	for _, r := range results {
		if f, ok := r.(*Failure); ok {
			return f // cancelled from outside this map_par
		}
	}
	return &List{Elems: results}
}

func builtinRead(in *Interp, c *callSite, args []Value) Value {
	path := in.strArg(c, "fs.read", args)
	if err := c.t.ctx.Err(); err != nil {
		return &Failure{Msg: "cancelled because another task in the same `together` failed", cancelled: true}
	}
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(in.dir, path)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		reason := "could not read the file"
		switch {
		case errors.Is(err, fs.ErrNotExist):
			reason = "file not found"
		case errors.Is(err, fs.ErrPermission):
			reason = "permission denied"
		default:
			var pe *fs.PathError
			if errors.As(err, &pe) {
				reason = pe.Err.Error()
			}
		}
		return &Failure{Msg: fmt.Sprintf("cannot read %s: %s", show(path), reason), Fallback: `""`}
	}
	return string(data)
}
