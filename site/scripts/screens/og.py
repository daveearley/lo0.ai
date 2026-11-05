"""Write og.html, the 1200x630 social card, into a copy of the built site.

usage: og.py SITE_COPY   (a copy of dist/, so the page's own styles and fonts load)

Serve the copy, screenshot og.html at 1200x630 and save it as public/og.png.
"""
import re
import sys

site = sys.argv[1]
h = open(site + "/index.html").read()
head = h[: h.index("</head>")]
styles = "".join(re.findall(r"<style[^>]*>.*?</style>", head, re.S))
links = "".join(re.findall(r"<link[^>]*(?:stylesheet|font)[^>]*>", head))
fig = re.search(r'<figure class="tui[^"]*".*?</figure>', h, re.S).group(0)
logo = re.search(r'<a class="home"[^>]*>(.*?)</a>', h, re.S).group(1)

card = """
html,body{margin:0;background:var(--bg)}
.og{width:1200px;height:630px;box-sizing:border-box;padding:56px 64px 0;display:grid;grid-template-columns:430px 1fr;gap:44px;overflow:hidden;
 background:radial-gradient(900px 500px at 85% 110%,color-mix(in oklch,var(--accent) 16%,transparent),transparent 70%),var(--bg)}
.og .copy{display:flex;flex-direction:column;gap:26px;padding-top:8px}
.og h1{font-size:58px;line-height:1.02;letter-spacing:-0.035em;margin:0}
.og p{font-size:21px;line-height:1.45;color:var(--ink-2);margin:0}
.og .cmd{font-family:var(--font-mono);font-size:16px;color:var(--ink);padding:12px 16px;border:1px solid var(--rule-strong);border-radius:10px;background:var(--surface);width:max-content}
.og .cmd b{color:var(--accent);font-weight:400}
.og .mark svg{height:42px;width:auto}
.og .shot{width:1180px;margin-top:22px}
"""
page = f"""<!doctype html><html lang="en-GB" data-theme="dark"><head><meta charset="utf-8">{links}{styles}<style>{card}</style></head>
<body><div class="og"><div class="copy"><span class="mark">{logo}</span>
<h1>Find the local model that fits your machine and your work.</h1>
<p>Shows which open models can do your work, and switches your coding agent over when you’re ready.</p>
<div class="cmd"><b>$</b> curl -fsSL lo0.ai/install.sh | sh</div></div>
<div class="shot">{fig}</div></div></body></html>"""
open(site + "/og.html", "w").write(page)
