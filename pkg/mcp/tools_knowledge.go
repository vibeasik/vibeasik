package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/vibeasik/vibeasik/pkg/knowledge"
	"github.com/vibeasik/vibeasik/pkg/models"
)

// Pusat Pengetahuan: satu pintu patokan & pengetahuan umum pekerja AI vibeasik. Tool membaca/menulis DB langsung
// sehingga jalan juga dari stdio.

func (s *Server) knowledgeToolsList() []Tool {
	return []Tool{
		{
			Name: "vibeasik_knowledge_recall",
			Description: "Searches vibeasik's Knowledge Hub — the single door for office-wide AI knowledge: guideline rubrics, audit checklists, lessons from audit verdicts, employee expertise and knowledge learned about new technologies. " +
				"Results are ranked by relevance (TF-IDF vectors + linked nodes), proven usefulness (helpful/harmful evidence), importance and recency; rules of other ecosystems are filtered out, and so are nodes that were superseded or are past their validity date. " +
				"A node's applies_to (e.g. 'go>=1.26') states where it holds; do not apply it outside that range. " +
				"Use it before working in an unfamiliar area. Project-specific facts are not here (see the project's executive board).",
			InputSchema: ToolSchema{Type: "object", Properties: map[string]interface{}{
				"query":       prop("string", "What you are working on, e.g. todo title + target files"),
				"domains":     prop("string", "Optional comma-separated ecosystems/areas, e.g. 'go,security'"),
				"employee_id": prop("integer", "Optional: include this employee's personal knowledge"),
				"limit":       prop("integer", "Max nodes (default 10)"),
			}, Required: []string{"query"}},
		},
		{
			Name: "vibeasik_knowledge_learn",
			Description: "Adds reusable, project-independent knowledge to the shared office library of the Knowledge Hub (e.g. how a new language/framework builds & tests, a pitfall you just hit). " +
				"A near-duplicate of an existing node reinforces it instead of duplicating. Do NOT store project-specific facts (paths, table names, features of one app). " +
				"When the new knowledge replaces an outdated node (e.g. a newer version changed the behaviour), pass supersedes: an outdated agent note or gap-research node stays stored but is no longer recalled; " +
				"a curated node (guideline, audit checklist, playbook, promoted, QA review, audit verdict, design gallery/theory) keeps being recalled and is flagged for review with your node named as its proposed replacement.",
			InputSchema: ToolSchema{Type: "object", Properties: map[string]interface{}{
				"title":       prop("string", "Short topic"),
				"content":     prop("string", "1-3 sentences of concrete, reusable knowledge"),
				"kind":        prop("string", "rule | procedure | lesson | concept (default lesson)"),
				"domain":      prop("string", "Ecosystem or area, e.g. go, web, flutter, gleam, security (auto-detected when empty)"),
				"applies_to":  prop("string", "Optional: where this holds, e.g. 'go>=1.26' or 'mcp 2025-06-18' (max 64 chars)"),
				"valid_until": prop("string", "Optional: date (YYYY-MM-DD) after which this should be reviewed and stops being recalled"),
				"supersedes":  prop("integer", "Optional: id of the outdated node this replaces (the number after K)"),
			}, Required: []string{"title", "content"}},
		},
		{
			Name:        "vibeasik_knowledge_feedback",
			Description: "Reports whether a Knowledge Hub node (label K<id> in your brief) helped or misled you on the current work. Evidence drives ranking; nodes proven harmful are pruned, guideline rules flagged for review.",
			InputSchema: ToolSchema{Type: "object", Properties: map[string]interface{}{
				"node_id": prop("integer", "Node id (the number after K)"),
				"helpful": prop("boolean", "true = helped, false = wrong/misleading"),
			}, Required: []string{"node_id", "helpful"}},
		},
		{
			Name: "vibeasik_knowledge_gap",
			Description: "Reports a knowledge gap: a technology, language or method the office does not know yet. The daemon turns open gaps into a learning agenda for the best-fit employee, whose results become shared knowledge nodes. " +
				"Ecosystems with zero nodes are detected automatically; use this for specific new methods inside known ecosystems too.",
			InputSchema: ToolSchema{Type: "object", Properties: map[string]interface{}{
				"topic":  prop("string", "What needs to be learned, e.g. 'Go 1.26 range-over-func iterators'"),
				"domain": prop("string", "Ecosystem/area (auto-detected when empty)"),
				"sample": prop("string", "Optional context where the gap appeared"),
			}, Required: []string{"topic"}},
		},
	}
}

// callKnowledgeTool returns handled=false when name is not a knowledge tool.
func (s *Server) callKnowledgeTool(name string, args map[string]interface{}) (*ToolCallResult, bool, error) {
	switch name {
	case "vibeasik_knowledge_recall":
		q := knowledge.Query{Text: argString(args, "query"), EmployeeID: argUint(args, "employee_id"), Limit: int(argUint(args, "limit"))}
		for _, d := range strings.Split(argString(args, "domains"), ",") {
			if d = strings.TrimSpace(d); d != "" {
				q.Domains = append(q.Domains, d)
			}
		}
		res, err := jsonResult(knowledge.Recall(s.db, q))
		return res, true, err
	case "vibeasik_knowledge_learn":
		note := knowledge.Note{
			Title: argString(args, "title"), Content: argString(args, "content"), Kind: argString(args, "kind"),
			Domain: argString(args, "domain"), Source: models.KnowledgeSrcAgent,
			AppliesTo: argString(args, "applies_to"), Supersedes: argUint(args, "supersedes"),
		}
		if raw := argString(args, "supersedes"); note.Supersedes == 0 && raw != "" { // "K12" juga diterima
			if note.Supersedes = argUint(map[string]interface{}{"v": strings.TrimLeft(raw, "Kk")}, "v"); note.Supersedes == 0 {
				return nil, true, fmt.Errorf("supersedes harus id node (mis. 12 atau K12), dapat %q", raw)
			}
		}
		// Lewat MCP, agent hanya boleh langsung menyembunyikan node tulisan agent; node lain cukup diusulkan.
		if note.Supersedes != 0 {
			var old models.KnowledgeNode
			if s.db.Where("id = ?", note.Supersedes).Limit(1).Find(&old).Error == nil && old.ID != 0 {
				note.ProposeSupersede = !knowledge.AgentMaySupersede(&old, note.OwnerEmployeeID)
			}
		}
		if v := strings.TrimSpace(argString(args, "valid_until")); v != "" {
			t, err := time.ParseInLocation("2006-01-02", v, time.Local)
			if err != nil {
				return nil, true, fmt.Errorf("valid_until harus tanggal YYYY-MM-DD: %w", err)
			}
			if !t.After(time.Now()) {
				return nil, true, fmt.Errorf("valid_until %s sudah lewat", v)
			}
			note.ValidUntil = &t
		}
		node, merged, err := knowledge.Learn(s.db, note)
		if err != nil && node == nil {
			return nil, true, err
		}
		if err != nil {
			res, jerr := jsonResult(map[string]interface{}{"node": node, "merged_into_existing": merged, "supersede_error": err.Error()})
			return res, true, jerr
		}
		out := map[string]interface{}{"node": node, "merged_into_existing": merged}
		if !merged {
			out["trial_note"] = "New shared knowledge from agents starts with a capped weight, so proven items rank above it until " +
				"a reviewer or a passing todo marks it helpful. It is retired only if it is judged wrong."
		}
		if note.ProposeSupersede {
			out["supersede_pending_review"] = true
			out["supersede_note"] = fmt.Sprintf("K%d is curated knowledge, so it was NOT replaced: it is still recalled and is now "+
				"flagged for review with K%d as the proposed replacement. The replacement waits for an owner's review.", note.Supersedes, node.ID)
		}
		res, err := jsonResult(out)
		return res, true, err
	case "vibeasik_knowledge_feedback":
		helpful, _ := args["helpful"].(bool)
		node, err := knowledge.Feedback(s.db, argUint(args, "node_id"), helpful)
		if err != nil {
			return nil, true, err
		}
		res, err := jsonResult(map[string]interface{}{"id": node.ID, "helpful": node.Helpful, "harmful": node.Harmful, "utility": node.Utility()})
		return res, true, err
	case "vibeasik_knowledge_gap":
		g, err := knowledge.ReportGap(s.db, argString(args, "domain"), argString(args, "topic"), argString(args, "sample"))
		if err != nil {
			return nil, true, err
		}
		res, err := jsonResult(g)
		return res, true, err
	}
	return nil, false, nil
}
