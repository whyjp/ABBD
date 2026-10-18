// Package mcp serves the Project Intelligence API over the Model Context Protocol stdio
// transport (newline-delimited JSON-RPC 2.0). Read-only views are resources, actions are
// tools; every tool calls the same workflow function as the CLI (FR-10).
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whyjp/abbd/internal/model"
	"github.com/whyjp/abbd/internal/ssot"
	"github.com/whyjp/abbd/internal/workflow"
)

const ProtocolVersion = "2025-11-25"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type args map[string]any

func (a args) str(k string) string {
	if v, ok := a[k].(string); ok {
		return v
	}
	return ""
}

func (a args) strs(k string) []string {
	var out []string
	if v, ok := a[k].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

type tool struct {
	Description string
	Props       map[string]string
	Required    []string
	Call        func(e *workflow.Engine, a args) (any, error)
}

func str(desc string) string { return desc }

// Tools is the MCP tool table of PRD ch.09 plus the contract and confirmation tools of 08A.
var Tools = map[string]tool{
	"project_status": {"Revision, freshness of maps/graph/HTML, pending changes, active turn.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Status() }},
	"resolve_intent": {"Dry-run: route the request, match features/modules/contracts, impact and verification commands. Writes nothing.",
		map[string]string{"request": str("user request")}, []string{"request"},
		func(e *workflow.Engine, a args) (any, error) { return e.Plan(a.str("request")) }},
	"start_turn": {"Start a development turn (session bootstrap, route, resolve intent, draft CHG).",
		map[string]string{"request": str("user request")}, []string{"request"},
		func(e *workflow.Engine, a args) (any, error) { return e.Run(a.str("request"), workflow.RunOptions{}) }},
	"get_feature_context": {"Feature purpose, contracts, entry points, verification and refs.",
		map[string]string{"id": str("FEAT-*")}, []string{"id"},
		func(e *workflow.Engine, a args) (any, error) { return e.Inspect(a.str("id")) }},
	"get_architecture_context": {"Module or interface responsibility, constraints, dependents and decision history.",
		map[string]string{"id": str("MOD-* or IF-*/EVT-*/DATA-*/CFG-*")}, []string{"id"},
		func(e *workflow.Engine, a args) (any, error) { return e.Inspect(a.str("id")) }},
	"trace_implementation": {"Bounded graph with edge evidence: a path between two IDs, or the neighborhood of one.",
		map[string]string{"from": str("start ID"), "to": str("optional end ID")}, []string{"from"},
		func(e *workflow.Engine, a args) (any, error) {
			if a.str("to") != "" {
				return e.Trace(a.str("from"), a.str("to"))
			}
			return e.Neighbors(a.str("from"), 2)
		}},
	"analyze_change_impact": {"Impact of the working tree: contracts first, modules, dependents, features, tests, unknowns.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Impact() }},
	"get_verification_plan": {"Commands, tests and expected behavior for the active change.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.VerificationPlan() }},
	"run_verification": {"Run affected (default) or all verification commands and store evidence.",
		map[string]string{"scope": str("affected|all")}, nil,
		func(e *workflow.Engine, a args) (any, error) {
			return e.Verify(workflow.VerifyOptions{All: a.str("scope") == "all"})
		}},
	"sync_project_knowledge": {"Completion gate: sync maps, graph and HTML after PASSED verification; returns the ChangeReceipt.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Sync(workflow.SyncOptions{}) }},
	"check_consistency": {"Schema, provenance rules, references, evidence links and stale views.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Check() }},
	"ask_user_decision": {"Record a tier B question (2-3 options + default). The host asks the user (prefer MCP elicitation); without an answer the default proceeds as an UNCONFIRMED ADR.",
		map[string]string{"question": str("question"), "options": "array of key=label=effect", "default": str("default option key"), "answer": str("answer key if the user replied"), "subjects": "array"},
		[]string{"question", "options"},
		func(e *workflow.Engine, a args) (any, error) {
			in := workflow.DecideInput{Tier: model.TierB, Question: a.str("question"), Default: a.str("default"), Answer: a.str("answer"), Subjects: a.strs("subjects")}
			for _, o := range a.strs("options") {
				in.Options = append(in.Options, ParseOption(o))
			}
			return e.Decide(in)
		}},
	"answer_decision": {"Show an ADR's state. An answer is refused here: answering an ADR records the user's decision, made on the review page (abbd serve), the decision file, or `abbd decide --adr ID --user` typed by the user.",
		map[string]string{"adr": str("ADR-*"), "answer": str("option key"), "reason": str("the user's reason")}, []string{"adr", "answer"},
		func(e *workflow.Engine, a args) (any, error) {
			return e.Decide(workflow.DecideInput{ADR: a.str("adr"), Answer: a.str("answer"), Reason: a.str("reason")})
		}},
	"record_tier_c_stop": {"Record a tier C stop (push, deploy, external deletion, secrets, breaking change with external consumers). Nothing runs until the user approves.",
		map[string]string{"question": str("what would run"), "subjects": "array"}, []string{"question"},
		func(e *workflow.Engine, a args) (any, error) {
			return e.Decide(workflow.DecideInput{Tier: model.TierC, Question: a.str("question"), Subjects: a.strs("subjects")})
		}},
	"get_external_contracts": {"External contract view (IF/EVT/DATA/CFG with owner, origin, verification).",
		map[string]string{"module": str("optional MOD-*")}, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Contracts(a.str("module")) }},
	"analyze_contract_impact": {"Contracts the working tree adds, removes or modifies, with their tier A/B/C.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.ContractsImpact() }},
	"list_review_cards": {"Structural change cards of a turn (default: the review target) with tier, status and the review page path. Show the page link to the user; the picture is there, not in chat.",
		map[string]string{"change": str("optional CHG-*")}, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Review(a.str("change")) }},
	"decide_structural_change": {"Decide one structural change card from the agent channel (approve | reject | revert | option:KEY, reason required for reject/revert). Tier B/C cards are refused here: they are the user's, decided on the review page (abbd serve) or the decision file.",
		map[string]string{"delta": str("CHG-x#n"), "decision": str("approve|reject|revert|option:KEY"), "reason": str("why")}, []string{"delta", "decision"},
		func(e *workflow.Engine, a args) (any, error) {
			return e.DecideDelta(workflow.DeltaDecisionInput{Delta: a.str("delta"), Decision: a.str("decision"), Reason: a.str("reason"), Channel: "mcp"})
		}},
	"list_unconfirmed": {"The confirm queue: UNCONFIRMED meaning fields, decisions and merge candidates.", nil, nil,
		func(e *workflow.Engine, a args) (any, error) { return e.Pending() }},
	"confirm_item": {"Confirm an item (optionally replacing its value) or reject it. Only the user's word promotes INFERRED.",
		map[string]string{"id": str("ID"), "value": str("replacement text"), "reject": "boolean"}, []string{"id"},
		func(e *workflow.Engine, a args) (any, error) {
			reject, _ := a["reject"].(bool)
			return e.Confirm(workflow.ConfirmInput{ID: a.str("id"), Value: a.str("value"), Reject: reject})
		}},
}

func (t tool) schema() map[string]any {
	props := map[string]any{}
	for k, d := range t.Props {
		switch d {
		case "array", "array of key=label=effect":
			props[k] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		case "boolean":
			props[k] = map[string]any{"type": "boolean"}
		default:
			props[k] = map[string]any{"type": "string", "description": d}
		}
	}
	s := map[string]any{"type": "object", "properties": props}
	if len(t.Required) > 0 {
		s["required"] = t.Required
	}
	return s
}

func Serve(e *workflow.Engine, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notification
		}
		result, rerr := handle(e, req)
		enc.Encode(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr})
	}
	return sc.Err()
}

func handle(e *workflow.Engine, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
			"serverInfo":      map[string]any{"name": "abbd", "version": "0.1.0-draft"},
			"instructions":    "ABBD project intelligence. Every answer carries revision and freshness; STALE or UNKNOWN means fall back to LSP/source.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		var list []map[string]any
		for _, name := range model.SortedKeys(Tools) {
			t := Tools[name]
			list = append(list, map[string]any{"name": name, "description": t.Description, "inputSchema": t.schema()})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments args   `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		t, ok := Tools[p.Name]
		if !ok {
			return nil, &rpcError{-32602, "unknown tool " + p.Name}
		}
		for _, r := range t.Required {
			if _, ok := p.Arguments[r]; !ok {
				return nil, &rpcError{-32602, "missing argument " + r}
			}
		}
		v, err := t.Call(e, p.Arguments)
		return toolResult(v, err), nil
	case "resources/list":
		return map[string]any{"resources": []map[string]any{
			{"uri": "abbd://status", "name": "project status", "mimeType": "application/json"},
			{"uri": "abbd://explorer", "name": "HTML Explorer", "mimeType": "text/html"},
			{"uri": "abbd://confirm-queue", "name": "confirm queue", "mimeType": "application/json"},
		}}, nil
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(req.Params, &p)
		return readResource(e, p.URI)
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

// ParseOption reads "key=label=effect" so every option says what it changes (PRD ch.10).
func ParseOption(s string) model.Option {
	parts := strings.SplitN(s, "=", 3)
	op := model.Option{Key: parts[0], Label: parts[0]}
	if len(parts) > 1 {
		op.Label = parts[1]
	}
	if len(parts) > 2 {
		op.Effect = parts[2]
	}
	return op
}

func toolResult(v any, err error) map[string]any {
	b, _ := json.MarshalIndent(map[string]any{"result": v}, "", "  ")
	res := map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}, "isError": err != nil}
	if err != nil {
		res["content"] = []map[string]any{{"type": "text", "text": fmt.Sprintf("error: %v\n%s", err, b)}}
	}
	return res
}

func readResource(e *workflow.Engine, uri string) (any, *rpcError) {
	var text, mime string
	switch uri {
	case "abbd://status":
		st, err := e.Status()
		if err != nil {
			return nil, &rpcError{-32603, err.Error()}
		}
		b, _ := json.MarshalIndent(st, "", "  ")
		text, mime = string(b), "application/json"
	case "abbd://confirm-queue":
		p, err := e.Pending()
		if err != nil {
			return nil, &rpcError{-32603, err.Error()}
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		text, mime = string(b), "application/json"
	case "abbd://explorer":
		b, err := os.ReadFile(ssot.Path(e.Root, "generated", "explorer.html"))
		if err != nil {
			return nil, &rpcError{-32603, err.Error()}
		}
		text, mime = string(b), "text/html"
	default:
		return nil, &rpcError{-32602, "unknown resource " + uri}
	}
	return map[string]any{"contents": []map[string]any{{"uri": uri, "mimeType": mime, "text": text}}}, nil
}
