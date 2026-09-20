package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const maxPacket = 1 << 20
const maxProtocolBytes = 32 << 20

type packet struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}
type outgoing struct {
	raw  []byte
	done chan error
}
type rpc struct {
	transport Transport
	mu        sync.Mutex
	pending   map[string]chan packet
	sequence  atomic.Uint64
	incoming  chan packet
	writes    chan outgoing
	done      chan struct{}
	once      sync.Once
}

func newRPC(transport Transport) *rpc {
	r := &rpc{transport: transport, pending: map[string]chan packet{}, incoming: make(chan packet, 128), writes: make(chan outgoing, 32), done: make(chan struct{})}
	go r.read()
	go r.write()
	return r
}
func (r *rpc) close() { r.once.Do(func() { close(r.done); _ = r.transport.Close() }) }
func (r *rpc) read() {
	defer r.close()
	scanner := bufio.NewScanner(io.LimitReader(r.transport, maxProtocolBytes+1))
	scanner.Buffer(make([]byte, 4096), maxPacket)
	total := 0
	serverIDs := map[string]bool{}
	for scanner.Scan() {
		raw := scanner.Bytes()
		total += len(raw) + 1
		if total > maxProtocolBytes {
			return
		}
		var p packet
		if json.Unmarshal(raw, &p) != nil || len(raw) == 0 {
			return
		}
		if p.Method != "" {
			if len(p.ID) > 0 {
				if !validRPCID(p.ID) || serverIDs[string(p.ID)] || len(serverIDs) >= 1024 {
					return
				}
				serverIDs[string(p.ID)] = true
			}
			if len(p.Result) > 0 || len(p.Error) > 0 {
				return
			}
			select {
			case r.incoming <- p:
			case <-r.done:
				return
			default:
				return
			}
			continue
		}
		if !validRPCID(p.ID) || (len(p.Result) == 0) == (len(p.Error) == 0) {
			return
		}
		r.mu.Lock()
		ch := r.pending[string(p.ID)]
		r.mu.Unlock()
		if ch == nil {
			return
		}
		select {
		case ch <- p:
		default:
			return
		}
	}
}
func validRPCID(raw json.RawMessage) bool {
	if len(raw) == 0 || len(raw) > 256 {
		return false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text != ""
	}
	var integer int64
	return json.Unmarshal(raw, &integer) == nil
}
func (r *rpc) write() {
	for {
		select {
		case <-r.done:
			return
		case out := <-r.writes:
			n, err := r.transport.Write(out.raw)
			if err == nil && n != len(out.raw) {
				err = io.ErrShortWrite
			}
			out.done <- err
			if err != nil {
				r.close()
				return
			}
		}
	}
}
func (r *rpc) send(ctx context.Context, p packet) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxPacket {
		return ErrProtocol
	}
	out := outgoing{raw: append(raw, '\n'), done: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return ErrUncertain
	case r.writes <- out:
	}
	select {
	case <-ctx.Done():
		r.close()
		return ErrUncertain
	case <-r.done:
		return ErrUncertain
	case err := <-out.done:
		return err
	}
}
func (r *rpc) call(ctx context.Context, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	id, _ := json.Marshal(fmt.Sprintf("reforge:%d", r.sequence.Add(1)))
	raw, err := json.Marshal(params)
	if err != nil {
		return ErrProtocol
	}
	ch := make(chan packet, 1)
	r.mu.Lock()
	r.pending[string(id)] = ch
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, string(id)); r.mu.Unlock() }()
	if err = r.send(ctx, packet{ID: id, Method: method, Params: raw}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		r.close()
		return ErrUncertain
	case <-r.done:
		return ErrUncertain
	case p := <-ch:
		if len(p.Error) > 0 {
			return ErrProtocol
		}
		if result != nil && json.Unmarshal(p.Result, result) != nil {
			return ErrProtocol
		}
		return nil
	}
}
func (r *rpc) notify(ctx context.Context, method string, params any) error {
	raw, _ := json.Marshal(params)
	return r.send(ctx, packet{Method: method, Params: raw})
}
func (r *rpc) reply(ctx context.Context, id json.RawMessage, result any) error {
	raw, _ := json.Marshal(result)
	return r.send(ctx, packet{ID: id, Result: raw})
}
func (r *rpc) reject(ctx context.Context, id json.RawMessage) error {
	return r.send(ctx, packet{ID: id, Error: json.RawMessage(`{"code":-32601,"message":"operation not permitted"}`)})
}
