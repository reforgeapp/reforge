package agent

import (
	"context"
	"encoding/json"
	"time"
)

func (c *Codex) pump(r *rpc) {
	requests := make(chan packet, 32)
	go func() {
		for {
			select {
			case <-r.done:
				return
			case p := <-requests:
				c.mu.Lock()
				ctx := c.runctx
				c.mu.Unlock()
				if ctx == nil {
					ctx = context.Background()
				}
				if err := c.handleRequest(ctx, p); err != nil {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					_ = r.reject(ctx, p.ID)
					cancel()
					c.finish("uncertain", err)
					r.close()
					return
				}
			}
		}
	}()
	for {
		select {
		case <-r.done:
			c.finish("uncertain", ErrUncertain)
			return
		case p := <-r.incoming:
			if len(p.ID) > 0 {
				if !validRPCID(p.ID) {
					c.finish("uncertain", ErrProtocol)
					r.close()
					return
				}
				c.mu.Lock()
				c.pendingRequests[string(p.ID)] = true
				c.mu.Unlock()
				select {
				case requests <- p:
				default:
					c.finish("uncertain", ErrProtocol)
					r.close()
					return
				}
			} else if err := c.notification(p); err != nil {
				c.finish("uncertain", err)
				r.close()
				return
			}
		}
	}
}
func (c *Codex) handleRequest(ctx context.Context, p packet) error {
	if p.Method == "item/tool/call" {
		return c.tool(ctx, p)
	}
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
	}
	if json.Unmarshal(p.Params, &params) != nil {
		return ErrProtocol
	}
	c.mu.Lock()
	valid := params.ThreadID == c.thread && params.TurnID == c.turn && c.turn != ""
	if valid {
		if c.approvals[string(p.ID)] {
			valid = false
		} else {
			c.approvals[string(p.ID)] = true
		}
	}
	c.mu.Unlock()
	if !valid {
		return ErrDenied
	}
	var response any
	switch p.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		response = map[string]string{"decision": "decline"}
	case "item/permissions/requestApproval":
		response = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
	default:
		return ErrDenied
	}
	if err := c.reply(ctx, p.ID, response); err != nil {
		return err
	}
	c.emit(Event{Type: "approval_denied", ID: params.ItemID, ApprovalID: string(p.ID)})
	return nil
}
func (c *Codex) notification(p packet) error {
	var params struct {
		ThreadID string  `json:"threadId"`
		TurnID   string  `json:"turnId"`
		ItemID   string  `json:"itemId"`
		Delta    string  `json:"delta"`
		AuthMode *string `json:"authMode"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Info json.RawMessage `json:"codexErrorInfo"`
			} `json:"error"`
		} `json:"turn"`
		Item struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"item"`
	}
	if json.Unmarshal(p.Params, &params) != nil {
		return ErrProtocol
	}
	if p.Method == "account/updated" {
		if params.AuthMode != nil && *params.AuthMode != "chatgpt" {
			return ErrDenied
		}
		c.mu.Lock()
		active := c.state == "running" || c.state == "starting"
		c.mu.Unlock()
		if active && params.AuthMode == nil {
			return ErrDenied
		}
		return nil
	}
	if p.Method == "account/login/completed" {
		var login struct {
			ID      string `json:"loginId"`
			Success bool   `json:"success"`
		}
		if json.Unmarshal(p.Params, &login) != nil {
			return ErrProtocol
		}
		c.mu.Lock()
		if c.state == "running" || c.state == "starting" || !c.authChanged {
			c.mu.Unlock()
			return ErrDenied
		}
		if login.ID != "" {
			c.loginStatusID = login.ID
			c.loginStatus = "failed"
			if login.Success {
				c.loginStatus = "completed"
			}
		}
		c.mu.Unlock()
		return nil
	}
	c.mu.Lock()
	thread, turn, state := c.thread, c.turn, c.state
	c.mu.Unlock()
	if params.ThreadID != "" && params.ThreadID != thread {
		return ErrProtocol
	}
	if params.TurnID != "" && turn != "" && params.TurnID != turn {
		return ErrProtocol
	}
	switch p.Method {
	case "turn/started":
		if params.ThreadID != thread || params.Turn.ID == "" || state != "starting" && state != "running" {
			return ErrProtocol
		}
		c.mu.Lock()
		if c.turn != "" && c.turn != params.Turn.ID {
			c.mu.Unlock()
			return ErrProtocol
		}
		c.turn = params.Turn.ID
		c.mu.Unlock()
	case "item/agentMessage/delta":
		if state != "starting" && state != "running" {
			return ErrProtocol
		}
		if !c.emit(Event{Type: "text_delta", ID: params.ItemID, Text: params.Delta}) {
			return ErrProtocol
		}
	case "turn/completed":
		c.mu.Lock()
		activeEffects := c.activeEffects + len(c.pendingRequests)
		c.mu.Unlock()
		if activeEffects > 0 {
			return ErrProtocol
		}
		if params.ThreadID != thread || params.Turn.ID == "" || turn != "" && params.Turn.ID != turn {
			return ErrProtocol
		}
		switch params.Turn.Status {
		case "completed":
			c.finish("completed", nil)
		case "interrupted":
			c.finish("canceled", nil)
		case "failed":
			kind := "failed"
			if params.Turn.Error != nil {
				var info string
				_ = json.Unmarshal(params.Turn.Error.Info, &info)
				if info == "usageLimitExceeded" || info == "sessionBudgetExceeded" {
					kind = "quota"
				}
			}
			c.finish(kind, ErrUncertain)
		default:
			return ErrProtocol
		}
	case "item/started", "item/completed":
		switch params.Item.Type {
		case "agentMessage", "userMessage", "reasoning", "plan", "dynamicToolCall":
		case "commandExecution", "fileChange":
			if p.Method == "item/completed" && params.Item.Status != "declined" {
				return ErrDenied
			}
		default:
			return ErrDenied
		}
	case "item/commandExecution/outputDelta", "item/fileChange/outputDelta", "item/tool/call":
		return ErrDenied
	case "thread/started", "thread/status/changed", "thread/tokenUsage/updated", "turn/diff/updated", "turn/plan/updated", "serverRequest/resolved", "account/rateLimits/updated", "item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded":
	case "error":
		return ErrUncertain
	default:
		return ErrProtocol
	}
	return nil
}

func (c *Codex) reply(ctx context.Context, id json.RawMessage, result any) error {
	c.mu.Lock()
	delete(c.pendingRequests, string(id))
	c.mu.Unlock()
	return c.rpc.reply(ctx, id, result)
}
