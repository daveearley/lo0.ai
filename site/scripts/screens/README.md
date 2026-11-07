# Screens

The screenshots on the home page are real lo0 output, captured from a demo history so no one's own sessions, spend or paths are published. Hardware and runtimes are whatever the machine running the capture has.

From the repository root:

```bash
python3 -m venv /tmp/lo0-venv && /tmp/lo0-venv/bin/pip install pyte pillow
make build
python3 site/scripts/screens/demo_home.py /tmp/lo0-demo     # 373 sessions across 7 fictional projects

# The TUI, as styled text in site/src/data/screens
py=/tmp/lo0-venv/bin/python cap=site/scripts/screens/capture.py out=site/src/data/screens
$py $cap bin/lo0 /tmp/lo0-demo $out 120 30 home:
$py $cap bin/lo0 /tmp/lo0-demo $out 120 36 models:3
$py $cap bin/lo0 /tmp/lo0-demo $out 120 36 try:enter

# lo0 web, light and dark, with headless Chrome
site/scripts/screens/web.sh bin/lo0 /tmp/lo0-demo site/src/assets/screens
```

`capture.py` runs `lo0 ui` in a pseudo-terminal, presses the keys given after each name and saves the screen as runs of text and colour. `TuiShot.astro` draws them as text, so they stay sharp and searchable.

For the social card, build the site, copy `dist/` somewhere, run `og.py` on the copy, serve it and screenshot `og.html` at 1200×630 into `public/og.png`.

The glossary has its own card. Run `og_glossary.py` on the same copy and screenshot `og-glossary.html` at 1200×630 into `public/og-glossary.png`. It shows the page's own context figure at 256k, so rebuild the card when the catalogue changes those models.
