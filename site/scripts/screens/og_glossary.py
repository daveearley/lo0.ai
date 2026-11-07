"""Write og-glossary.html, the glossary's 1200x630 social card, into a copy of the built site.

usage: og_glossary.py SITE_COPY   (a copy of dist/, so the page's own styles and fonts load)

Serve the copy, screenshot og-glossary.html at 1200x630 and save it as
public/og-glossary.png. The figure is the page's own, set to 256k.
"""
import re
import sys

site = sys.argv[1]
h = open(site + "/glossary.html").read()
head = h[: h.index("</head>")]
styles = "".join(re.findall(r"<style[^>]*>.*?</style>", head, re.S))
links = "".join(re.findall(r"<link[^>]*(?:stylesheet|font)[^>]*>", head))
logo = re.search(r'<a class="home"[^>]*>(.*?)</a>', h, re.S).group(1)
terms = re.search(r"Glossary · (\d+) terms", h).group(1)

fig = re.search(r'<figure class="fig[^"]*" id="fig-kv-cache".*?</figure>', h, re.S).group(0)
fig = re.sub(r"<p class=\"note\".*?</p>", "", fig, flags=re.S)
fig = fig.replace(" checked", "")
fig = re.sub(r'(name="ctx-len" value="256")', r"\1 checked", fig)

card = """
html,body{margin:0;background:var(--bg)}
.og{width:1200px;height:630px;box-sizing:border-box;padding:56px 0 0 64px;display:grid;grid-template-columns:400px 1fr;grid-template-rows:574px;gap:40px;overflow:hidden;background:var(--bg)}
.og .copy{display:flex;flex-direction:column;gap:24px;padding-top:8px}
.og .mark svg{height:42px;width:auto}
.og .kicker{font-family:var(--font-mono);font-size:15px;color:var(--accent);margin:18px 0 -8px}
.og h1{font-size:64px;line-height:1;letter-spacing:-0.035em;margin:0}
.og p.lede{font-size:21px;line-height:1.45;color:var(--ink-2);margin:0}
.og .url{margin-top:auto;margin-bottom:56px;font-family:var(--font-mono);font-size:16px;color:var(--ink-3)}
.og .url b{color:var(--ink);font-weight:400}
.og .shot{min-width:0;padding-top:4px}
.og .shot figure{margin:0;border-top-right-radius:0;border-bottom-right-radius:0;border-right:0}
.og .shot *{animation:none!important;transition:none!important}
"""
page = f"""<!doctype html><html lang="en-GB" data-theme="dark"><head><meta charset="utf-8">{links}{styles}<style>{card}</style></head>
<body><div class="og"><div class="copy"><span class="mark">{logo}</span>
<p class="kicker">Glossary · {terms} terms</p>
<h1>The local AI glossary</h1>
<p class="lede">What 30B-A3B, Q4_K_M and KV cache mean, and what each costs on your machine, worked out from real model data.</p>
<p class="url"><b>lo0.ai</b>/glossary</p></div>
<div class="shot">{fig}</div></div></body></html>"""
open(site + "/og-glossary.html", "w").write(page)
