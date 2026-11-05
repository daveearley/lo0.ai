package report

import (
	"time"

	"lo0.ai/lo0/internal/claudecode"
	"lo0.ai/lo0/internal/claudecode/install"
	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/usage"
	"lo0.ai/lo0/internal/version"
)

const maxSkillsShown = 4

// workRows describe how requests unfold and what they produce, under the
// session history.
func workRows(d *doc, p usage.Profile) {
	if r := p.Requests; r.Count > 0 {
		d.row("Requests", d.join(
			format.Int(r.Count),
			"median "+format.Plural(r.ToolCalls.P50, "tool call", "tool calls"),
			"1 in 10 over "+d.t.Accent.Render(format.Int(r.ToolCalls.P90)),
		))
		d.more(d.t.Muted.Render(d.join(
			"median wait "+format.Duration(time.Duration(r.Seconds.P50)*time.Second),
			"median "+format.Tokens(r.Output.P50)+" tokens written",
		)))
	}
	if r := p.Reasoning; r.HighEffortShare > 0 || r.ThinkingTokens > 0 {
		d.row("Thinking", d.join(
			format.Percent(r.HighEffortShare)+" of turns at high effort",
			d.t.Muted.Render(format.Percent(r.ThinkingShare)+" of output spent thinking"),
		))
	}
	if img := p.Images; img.Sessions > 0 {
		d.row("Images", d.join(
			format.Plural(img.Sessions, "session", "sessions"),
			d.t.Muted.Render(format.Int(img.Attached)+" pasted, "+format.Int(img.FromTools)+" from tools"),
		))
	}
	if c := p.Changes; c.LinesAdded+c.LinesRemoved > 0 {
		d.row("Changes", d.join(
			d.t.Good.Render("+"+format.Int(c.LinesAdded))+" "+d.t.Bad.Render("−"+format.Int(c.LinesRemoved))+" lines",
			format.Plural(c.Files, "file", "files"),
			format.Plural(c.PullRequests, "pull request", "pull requests"),
		))
	}
	if e := p.ToolErrors; e.Results > 0 {
		d.row("Tool errors", d.join(
			format.Percent(e.Share)+" of tool calls failed",
			d.t.Muted.Render(format.Int(e.Failed)+" of "+format.Int(e.Results)),
		))
	}
	interruptions(d, p.Interruptions)
	if len(p.Skills) > 0 {
		d.row("Skills", d.fit(counts(d, p.Skills[:min(len(p.Skills), maxSkillsShown*2)])))
	}
	if len(p.Entrypoints) > 0 {
		items := make([]string, 0, len(p.Entrypoints))
		for _, e := range p.Entrypoints {
			items = append(items, entrypointName(e.Name)+" "+d.t.Muted.Render(format.Int(e.Sessions)))
		}
		d.row("Started from", d.fit(items))
	}
	var versions []string
	for _, h := range byHarness(p) {
		f := h.Profile.Format
		if len(f.Versions) == 0 {
			continue
		}
		span := f.Newest()
		if f.Oldest() != f.Newest() {
			span = f.Oldest() + " to " + f.Newest()
		}
		versions = append(versions, h.Name+" "+span+d.t.Muted.Render(" ("+format.Plural(len(f.Versions), "version", "versions")+")"))
	}
	for i, v := range versions {
		if i == 0 {
			d.row("Versions", v)
		} else {
			d.more(v)
		}
	}
}

// byHarness lists each harness's own profile; an unpooled profile is Claude
// Code's.
func byHarness(p usage.Profile) []usage.HarnessProfile {
	if len(p.Harnesses) == 0 {
		return []usage.HarnessProfile{{Name: claudecode.Harness, Profile: p}}
	}
	return p.Harnesses
}

// harnessRows say how the pooled sessions split between harnesses, and
// which harness caps its sessions at its own window, so a pooled fit is not
// read as more than it is.
func harnessRows(d *doc, p usage.Profile) {
	var split []string
	for _, h := range p.Harnesses {
		if h.Profile.Sessions > 0 {
			split = append(split, h.Name+" "+d.t.Muted.Render(format.Int(h.Profile.Sessions)))
		}
	}
	if len(split) < 2 {
		return
	}
	d.more(d.join(split...))
	for _, h := range p.Harnesses {
		if w := h.Profile.ContextWindow; w > 0 && h.Profile.Sessions > 0 {
			d.prose("", d.t.Muted.Render(h.Name+" compacts at its own "+format.Tokens(w)+" window, so its sessions never get larger; their work past that point is not in these numbers."))
		}
	}
}

// entrypoints are the names Claude Code gives the places a session started
// from. Names lo0 does not know are shown as they are.
var entrypoints = map[string]string{
	"cli":            "terminal",
	"claude-desktop": "Claude app",
	"claude-vscode":  "VS Code",
	"sdk-ts":         "Agent SDK",
	"sdk-py":         "Agent SDK",
	"sdk-cli":        "Agent SDK",
	"codex_cli_rs":   "Codex CLI",
}

func entrypointName(e string) string {
	if name, ok := entrypoints[e]; ok {
		return name
	}
	return e
}

// interruptions lists what stopped the hosted models mid-work. A local model
// has no usage limits and needs no network, so each of these is a reason to
// switch, said plainly.
func interruptions(d *doc, in usage.Interruptions) {
	var parts []string
	if in.LimitHits > 0 {
		parts = append(parts, d.t.Warn.Render(format.Plural(in.LimitHits, "usage limit", "usage limits"))+
			d.t.Muted.Render(" in "+format.Plural(in.LimitSessions, "session", "sessions")))
	}
	if in.FailedResponses > 0 {
		parts = append(parts, format.Plural(in.FailedResponses, "failed response", "failed responses"))
	}
	retries := ""
	if in.Retries > 0 {
		retries = format.Plural(in.Retries, "retry", "retries")
		if in.NetworkRetries > 0 {
			retries += ", " + format.Int(in.NetworkRetries) + " after a dropped connection"
		}
	}
	switch {
	case len(parts) > 0:
		d.row("Interrupted", d.join(parts...))
		if retries != "" {
			d.more(d.t.Muted.Render(retries))
		}
	case retries != "":
		d.row("Interrupted", retries)
	}
}

// claudeRow says which Claude Code builds lo0 found and how they compare
// with the newest release.
func claudeRow(d *doc, in install.Installs, history usage.Format) {
	var parts []string
	if in.App != "" {
		parts = append(parts, in.App+d.t.Muted.Render(" in the Claude app"))
	}
	if in.CLI != "" {
		cli := in.CLI + d.t.Muted.Render(" on PATH")
		if in.App != "" && version.Compare(in.CLI, in.App) < 0 {
			cli = d.t.Warn.Render(in.CLI) + d.t.Muted.Render(" on PATH, behind the app")
		}
		parts = append(parts, cli)
	}
	if len(parts) == 0 && history.Newest() != "" {
		parts = append(parts, history.Newest()+d.t.Muted.Render(" in your history"))
	}
	if len(parts) == 0 {
		return
	}
	d.row("Claude Code", d.join(parts...))
	if in.Latest != "" {
		latest := "newest " + in.Latest
		if in.Stable != "" && in.Stable != in.Latest {
			latest += ", stable " + in.Stable
		}
		if version.Compare(install.Newest(in, history), in.Latest) >= 0 {
			latest += ", which you have"
		}
		d.more(d.t.Muted.Render(d.join(latest, "lo0 checked against "+claudecode.VerifiedVersion)))
	}
}
