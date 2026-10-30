package interp

import (
	"context"
	"sync"

	"github.com/kirannarayanak/fennic/ast"
)

// group is the set of tasks a `together` block has spawned. The block does
// not finish until every one of them has. The first task to fail cancels
// the others, and its failure becomes the block's.
type group struct {
	wg     sync.WaitGroup
	mu     sync.Mutex
	cause  *Failure // the first real failure; cancellations never count
	panics []any    // runtime errors in tasks, re-raised by the block
	cancel context.CancelFunc
}

func (g *group) failed(f *Failure) {
	g.mu.Lock()
	if g.cause == nil && !f.cancelled {
		g.cause = f
	}
	g.mu.Unlock()
	g.cancel()
}

// evalTogether runs a block as a structured scope. Its result is a failure
// if the block or any task it spawned failed, which `!` or `else` then
// handle. No task outlives it.
func (in *Interp) evalTogether(e *ast.TogetherExpr, env *Env, t *task) (result Value) {
	ctx, cancel := context.WithCancel(t.ctx)
	g := &group{cancel: cancel}
	inner := &task{ctx: ctx, cancel: cancel, together: true, group: g}
	defer func() {
		r := recover()
		if r != nil {
			cancel() // the block stopped early: stop its tasks too
		}
		g.wg.Wait()
		cancel()
		if len(g.panics) > 0 {
			panic(g.panics[0])
		}
		if r != nil {
			p, ok := r.(*propagation)
			if !ok {
				panic(r)
			}
			result = p.failure
		}
		if g.cause != nil {
			result = g.cause
		} else if f, ok := result.(*Failure); ok && f.cancelled {
			result = f
		}
	}()
	return in.evalBlock(e.Body, env, inner)
}

// spawn starts fn as a task of the enclosing `together`.
func (in *Interp) spawn(e *ast.SpawnExpr, fn Value, t *task) Value {
	g := t.group
	if g == nil {
		in.fail(e, "put it inside a `together { ... }` block", "`spawn` must run inside a `together` block")
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				if p, ok := r.(*propagation); ok {
					g.failed(p.failure)
					return
				}
				g.mu.Lock()
				g.panics = append(g.panics, r)
				g.mu.Unlock()
				g.cancel()
			}
		}()
		if f, failed := in.call(fn, nil, &callSite{node: e, t: t}).(*Failure); failed {
			g.failed(f)
		}
	}()
	return Unit
}

// Channel is a `Chan[T]`. Sending and receiving give up when the task's
// `together` is cancelled.
type Channel struct {
	ch     chan Value
	mu     sync.Mutex
	closed bool
}

var cancelledFailure = &Failure{Msg: "cancelled because another task in the same `together` failed", cancelled: true}

func (in *Interp) send(c *Channel, v Value, site *callSite) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		in.fail(site.node, "send every value before calling `.close()`", "cannot send on a channel that is closed")
	}
	select {
	case c.ch <- v:
	case <-site.t.ctx.Done():
		panic(&propagation{failure: cancelledFailure, at: site.node})
	}
}

func (in *Interp) closeChan(c *Channel, site *callSite) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		in.fail(site.node, "close a channel once, after the last send", "this channel is already closed")
	}
	c.closed = true
	close(c.ch)
}

// receive gives the next value, or false when the channel is closed and
// empty, or the task is cancelled.
func (c *Channel) receive(ctx context.Context) (Value, bool) {
	select {
	case v, ok := <-c.ch:
		return v, ok
	case <-ctx.Done():
		return nil, false
	}
}
