"""Write a synthetic HOME with Claude Code history for lo0 website screenshots.

The projects, prompts and people are fictional. Token usage follows the shape
of a heavy Claude Code user: median session peaks near 180k tokens, one in ten
passes 500k.
"""
import json
import math
import os
import random
import shutil
import sys
import uuid
from datetime import datetime, timedelta, timezone

home = sys.argv[1]
rng = random.Random(7)

if os.path.exists(home):
    shutil.rmtree(home)
claude = os.path.join(home, ".claude")
os.makedirs(os.path.join(claude, "projects"))

projects = [
    ("payments-api", 0.30, ["go"]),
    ("web-dashboard", 0.22, ["tsx", "ts", "css"]),
    ("mobile-app", 0.14, ["swift"]),
    ("infra", 0.11, ["tf", "yaml"]),
    ("data-pipeline", 0.10, ["py"]),
    ("docs-site", 0.07, ["md", "astro"]),
    ("cli-tools", 0.06, ["rs"]),
]

for name, _, _ in projects:
    p = os.path.join(home, "code", name)
    os.makedirs(os.path.join(p, ".claude", "commands"))
    with open(os.path.join(p, "CLAUDE.md"), "w") as f:
        f.write(f"# {name}\n\nRun the tests before committing.\n")
    with open(os.path.join(p, ".claude", "commands", "ship.md"), "w") as f:
        f.write("Run the checks, write the changelog line and open a pull request.\n")
    servers = {"payments-api": "postgres", "data-pipeline": "postgres", "web-dashboard": "playwright"}
    if name in servers:
      with open(os.path.join(p, ".mcp.json"), "w") as f:
        json.dump({"mcpServers": {servers[name]: {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-" + servers[name]], "env": {"DATABASE_URL": "x"}}}}, f)

with open(os.path.join(claude, "CLAUDE.md"), "w") as f:
    f.write("Prefer small commits. British spelling in docs.\n")
os.makedirs(os.path.join(claude, "skills", "release-notes"))
with open(os.path.join(claude, "skills", "release-notes", "SKILL.md"), "w") as f:
    f.write("---\nname: release-notes\ndescription: Draft release notes from merged PRs\n---\nRead the merged PRs and draft notes.\n")
os.makedirs(os.path.join(claude, "agents"))
with open(os.path.join(claude, "agents", "reviewer.md"), "w") as f:
    f.write("---\nname: reviewer\ndescription: Reviews a diff for bugs\n---\nReview the diff.\n")
with open(os.path.join(claude, "settings.json"), "w") as f:
    json.dump({
        "permissions": {"allow": ["Bash(go test:*)", "Bash(npm run:*)", "Read"], "deny": ["Bash(rm -rf:*)"]},
        "hooks": {
            "PostToolUse": [{"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "gofmt -w ."}]}],
            "Stop": [{"hooks": [{"type": "command", "command": "afplay /System/Library/Sounds/Glass.aiff"}]}],
        },
        "model": "opus",
    }, f, indent=2)
with open(os.path.join(home, ".claude.json"), "w") as f:
    json.dump({
        "oauthAccount": {"organizationType": "claude_max", "organizationRateLimitTier": "default_claude_max_5x", "hasExtraUsageEnabled": False},
        "mcpServers": {
            "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/", "headers": {"Authorization": "x"}},
            "linear": {"type": "http", "url": "https://mcp.linear.app/mcp"},
            "sentry": {"type": "http", "url": "https://mcp.sentry.dev/mcp"},
        },
        "projects": {os.path.join(home, "code", n): {} for n, _, _ in projects},
    }, f, indent=2)

start = datetime(2026, 5, 4, tzinfo=timezone.utc)
end = datetime(2026, 11, 4, 18, tzinfo=timezone.utc)
days = (end - start).days


def model_for(t):
    if t < datetime(2026, 7, 20, tzinfo=timezone.utc):
        return "claude-opus-5"
    r = rng.random()
    if r < 0.12:
        return "claude-sonnet-5-5"
    return "claude-opus-5-5"


def peak_tokens():
    # Log-normal around 175k with a long tail, capped at the 1M window.
    v = math.exp(rng.gauss(math.log(175_000), 0.95))
    return int(min(max(v, 22_000), 990_000))


tools_by_lang = {
    "go": ["Read", "Edit", "Bash", "Grep", "Read", "Edit", "Bash"],
    "tsx": ["Read", "Edit", "Bash", "Glob", "Read", "Edit"],
    "swift": ["Read", "Edit", "Bash", "Read"],
}
mcp_tools = ["mcp__github__create_pull_request", "mcp__github__get_pull_request", "mcp__linear__get_issue",
             "mcp__sentry__get_issue", "mcp__postgres__query"]

sessions = 0
for d in range(days):
    day = start + timedelta(days=d)
    if day.weekday() >= 5 and rng.random() < 0.7:
        continue
    # Usage grows over the months.
    per_day = rng.randint(1, 3) + (1 if d > days * 0.5 else 0) + (1 if d > days * 0.8 and rng.random() < 0.5 else 0)
    for _ in range(per_day):
        r = rng.random()
        acc = 0
        for name, share, langs in projects:
            acc += share
            if r <= acc:
                break
        cwd = os.path.join(home, "code", name)
        sid = str(uuid.UUID(int=rng.getrandbits(128)))
        t = day + timedelta(hours=rng.uniform(8, 19))
        model = model_for(t)
        peak = peak_tokens()
        responses = max(12, int(peak / 4_500 * rng.uniform(0.6, 1.3)))
        enc = cwd.replace("/", "-").replace(".", "-")
        pdir = os.path.join(claude, "projects", enc)
        os.makedirs(pdir, exist_ok=True)
        lines = []
        base = {"cwd": cwd, "sessionId": sid, "version": "2.1.317", "entrypoint": "cli", "isSidechain": False}

        def rec(**kw):
            r = dict(base)
            r.update(kw)
            r["timestamp"] = t.strftime("%Y-%m-%dT%H:%M:%S.") + f"{t.microsecond // 1000:03d}Z"
            lines.append(json.dumps(r))

        rec(type="user", message={"role": "user", "content": "prompt"}, parentUuid=None)
        ctx = rng.randint(14_000, 21_000)
        chain = 0
        chain_len = rng.randint(3, 40)
        lang = rng.choice(langs)
        for i in range(responses):
            frac = (i + 1) / responses
            target = int(18_000 + (peak - 18_000) * frac ** 0.9)
            grow = max(target - ctx, 200)
            out = int(rng.uniform(120, 1_400))
            chain += 1
            end_turn = chain >= chain_len or i == responses - 1
            if end_turn:
                content = [{"type": "text", "text": "x"}]
            else:
                pool = tools_by_lang.get(lang, ["Read", "Edit", "Bash", "Grep"])
                tool = rng.choice(pool) if rng.random() > 0.08 else rng.choice(mcp_tools)
                if rng.random() < 0.02:
                    tool = "Agent"
                if rng.random() < 0.01:
                    tool = "Skill"
                inp = {}
                if tool in ("Read", "Edit"):
                    inp = {"file_path": f"{cwd}/src/file{rng.randint(1, 40)}.{lang}"}
                if tool == "Skill":
                    inp = {"skill": "release-notes"}
                if tool == "Agent":
                    inp = {"subagent_type": "reviewer"}
                content = [{"type": "tool_use", "id": f"t{i}", "name": tool, "input": inp}]
            t += timedelta(seconds=rng.uniform(1.2, 4.5) + out / 160)
            rec(type="assistant", message={
                "id": f"msg_{sid[:8]}_{i}", "model": model, "content": content,
                "stop_reason": "end_turn" if end_turn else "tool_use",
                "usage": {"input_tokens": rng.randint(1, 8), "cache_creation_input_tokens": grow,
                          "cache_read_input_tokens": ctx, "output_tokens": out,
                          "cache_creation": {"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": grow}},
            })
            ctx += grow
            if end_turn:
                chain = 0
                chain_len = rng.randint(3, 40)
                t += timedelta(seconds=rng.uniform(20, 300))
                rec(type="user", message={"role": "user", "content": "prompt"})
            else:
                t += timedelta(seconds=rng.uniform(0.3, 4))
                rec(type="user", message={"role": "user", "content": [{"type": "tool_result", "tool_use_id": f"t{i}", "content": "ok"}]})
        if rng.random() < 0.18:
            t += timedelta(seconds=10)
            rec(type="pr-link", prUrl=f"https://github.com/acme/{name}/pull/{rng.randint(100, 900)}")
        with open(os.path.join(pdir, sid + ".jsonl"), "w") as f:
            f.write("\n".join(lines) + "\n")
        sessions += 1

print(f"{sessions} sessions in {home}")
