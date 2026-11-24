# whatnow

A dashboard for running several Claude Code sessions at once: see what each one does and which of them waits for
you, follow the progress of large, multi-project features stage by stage, and correct or ask a session from the
page, switching between features in one click.

It is made for engineers who run several features in parallel with AI coding agents in the terminal and want
one place to look, instead of reading long CLI transcripts. It shows progress, not tokens or costs.

## What you see
- **Home page**: one card per feature. The percent of its plan that is done, the stages of the plan as a strip
  (with an icon for planning, research, code, tests, review, deployment, docs), problems, when it was last updated, the next step, and whether
  a session is working on it right now.
- **Feature page** (tabs: Progress, What is left, Decisions and questions): what was planned
  and what really happened, stage by stage; what is left and who has to act on it.
- **Buttons on every item**: *Do it*, *Ask*, *Not needed*. They send your instruction or question into the
  Claude Code session that works on the feature (see below).

## Requirements
- Claude Code (tested with 2.1.329) and Go 1.26+ to build. The result is one program of about 10 MB.
- The desktop window uses the system WebView: WebKit on macOS, WebView2 on Windows (part of Windows 10 and 11),
  WebKitGTK on Linux (`libwebkit2gtk-4.1-dev`; without it `install.sh` builds the commands and the server only).
- Windows: run `install.sh` in Git Bash (Claude Code on Windows uses it too). Autostart there is an entry in the
  Run key of the user's registry.

## Install
Ready builds for macOS (one app for Apple Silicon and Intel), Windows and Linux (amd64, arm64) are made by the
`build` workflow in GitHub Actions and attached to a release for every `v*` tag. To build it yourself:
```sh
./install.sh        # copies skill/ into ~/.claude/skills/whatnow and builds ~/.claude/skills/whatnow/whatnow
```
Then ask Claude to start a dashboard for a big piece of work (the `whatnow` skill does this). It writes
`~/.whatnow/features/<name>/feature.json` and builds the pages. Open them:

```sh
~/.claude/skills/whatnow/whatnow                    # the desktop window
~/.claude/skills/whatnow/whatnow serve 7777         # the same pages for a browser: http://127.0.0.1:7777/
~/.claude/skills/whatnow/whatnow render --index --open   # static files, rebuilt on demand
```

## How it works
```
feature.json  ->  whatnow render  ->  panel.html (feature page) + index.html (home page)
```
- The source of truth is one `feature.json` per feature. Claude writes it while it works (stages,
  blockers, evidence) following the rules in `skill/SKILL.md`. The format is described in `skill/SCHEMA.md`.
- `whatnow render` validates the data and **derives** everything that must not be typed by hand: stage colours,
  counts, percentages, "ready to release?", staleness. It refuses a "passed" without a note saying what
  was checked, and a blocker that points to a stage that does not exist.
- The percent comes from the stages of the feature's own plan: every stage weighs the same and counts by the
  share of its items that are done (partly = 0.5). Nothing is imposed on a feature: tests, review or deployment
  are stages or items only where its plan has them.
- A feature with no update for 3 days is marked as not updated recently.

## Live page, hooks and server
A static `index.html` only changes when it is rebuilt. For a page that updates itself and shows where work is
happening, there are two parts:

```
Claude Code hooks -> whatnow hook -> ~/.whatnow/sessions/*.json + events.jsonl -> whatnow serve / window -> page (polls every 3 s)
```
- `whatnow hook` is run by Claude Code on session events. It is silent, never blocks and always exits 0. It records
  the state of each session and the `git commit`, `git push` and test-run events (other commands are dropped).
  The text of your prompts is never stored.
- `whatnow serve` serves `http://127.0.0.1:7777/` and re-reads `feature.json` and the hook data on every request.
  It listens on 127.0.0.1 only. The desktop window shows the same pages without going through the network, and also
  serves them on the port when the port is free.
- A session is linked to a feature, never a branch or folder. It links itself the moment it touches the feature's files (edits `feature.json`, runs `whatnow render` on it) or takes a message from the dashboard. You can also click **Attach a session** in the header of the feature page and pick one from the list, open or closed. Links live in `~/.whatnow/bindings.json`.
  A session without a feature is shown in its project's card area as a session without a feature.

Install the hooks (backup, merge with your existing hooks, safe to repeat):
```sh
~/.claude/skills/whatnow/whatnow hooks install     # --dry-run shows the diff; "hooks uninstall" removes them
~/.claude/skills/whatnow/whatnow hooks print       # just print the hooks block
```
The hooks are: state tracking (async) on session start, prompt, Bash and file-edit tool use, stop, notification and session end;
a synchronous `--deliver` hook on prompt submit; and `--watch` hooks (`asyncRewake`) on stop and session start.
Hooks of an older install (`python3 .../hook.py`) are replaced. A `--nudge` hook on stop reminds a session that finished a turn without updating its feature file. It never chains (it skips turns caused by a stop hook), waits 30 minutes between reminders to one session, and can be turned off with `"nudge": false` in `~/.whatnow/config.json`.
`docs/hooks-settings.example.json` has a copy.

## The Sessions tab
The home page opens on **Sessions**: every open Claude Code session, grouped by project, newest first, whether or not it belongs to a feature. The list comes from `claude agents`, and what each session is about comes from its transcript (how it started, what you asked last, what Claude answered last), so a session shows up even before it has triggered a hook. Per session: **Ask** (sends a message to that session; a closed session is resumed in the background), **Copy resume command**, **Attach to a feature**, and **Hide** for sessions that do not belong on the page (hidden ids are kept in `~/.whatnow/hidden.json`). Click the star on a session or a feature to put it in the **Favorites** tab (kept in `~/.whatnow/favorites.json`); a starred session stays there after it is closed. The **Features** tab is the portfolio of large features.

## Buttons on the feature page
On a feature page opened through the server every item of a stage has actions:
- **Do it**: a task. The text goes to the session that works on this feature.
- **Ask**: your question about the item. The answer comes back in the same session.
- **Not needed**: a decision with a reason. It applies at once: the item is struck through and stops counting as
  unfinished. It is stored in `overrides.json` next to `feature.json` (Claude never writes that file, so the
  two cannot overwrite each other). It follows the item's text, not its position. **Restore** undoes it.

**Where *Do it* and *Ask* go.** The message is written to `~/.whatnow/inbox/` and routed:
1. **A session of the feature is open** (confirmed by `claude agents --json`): the message is addressed to it.
   If it is idle, the `--watch` hook wakes it (`asyncRewake`: exit code 2 makes Claude Code start a turn).
   If it is busy, the message arrives when it finishes its answer. Without the watcher it arrives at your next prompt.
2. **Only a closed session is known:** the server runs `claude --resume <id> --bg "<message>"`. The session continues
   in the background under the same id with its old context. Watch it with `claude attach <id>`.
3. **No session is known:** the message waits for the first session that starts working on the feature.

**Request status.** The header of a feature page has a **↻ Request status** button next to "Updated ... ago". It asks the
feature's session (same routing as above) to compare `feature.json` with what it really did and bring the page up to
date. Only one such request is open at a time; the page reloads itself when the file changes.

**The bar on the left.** Every live page has a bar on the left: **Sessions**, **Features** and **Favorites** (the
views of the home page, which has no tabs of its own), then all features in groups: favorites, the ones a session works on right now, the rest, finished ones.
One click opens another feature. The dot and the line under a name show what its session does. A feature whose
session wrote something and now waits for your answer is marked with 💬 (✋ when Claude asks for a permission).
While you have not opened that feature since the reply, the row is highlighted, the mark has a red dot, and the
browser tab title starts with the number of such features. "N items for you" counts the open items of the feature that only you can do. The arrow at the
top collapses the bar to a narrow strip (remembered); on a narrow screen it becomes a row of chips.

**Last reply and chat.** A feature page shows the latest reply of the feature's session as one line, under the tabs
and in the bottom right corner (that one stays in view while you scroll). **Reply** on either opens a chat window:
the recent conversation of the session read from its transcript (what you typed in the terminal, what Claude answered)
and a field to write to it. A message goes to that session like one from the Sessions tab, and its delivery state
shows under it. A red dot marks a reply you have not opened yet. Drag the top or left edge of the window to resize it
(double click an edge to reset); **Full width** stretches it along the bottom of the screen. The size is remembered.

**Background task running.** A session that ended its turn while a background command or agent is still running does
not wait for you: it goes on by itself when the task ends. Claude Code reports such a session as busy, and the pages
show it as "background task running" instead of "waiting for you".

When Claude has handled a message it runs `whatnow inbox resolve <id> "summary"` and the summary shows on the page.

**Safety.** The page accepts writes only from the server's own page: `Host` and `Origin` are checked, the body must be
JSON, a custom `X-Whatnow` header is required (a site in your browser cannot send it without a CORS preflight, which
the server never allows) and the body size is limited. `claude` is started without a shell: your text is passed as one
argument and is never concatenated into a command.

## Autostart
```sh
~/.claude/skills/whatnow/whatnow autostart install      # --dry-run; also "autostart status", "autostart uninstall"
```
- macOS: a LaunchAgent `~/Library/LaunchAgents/whatnow.server.plist`. Linux: a systemd user unit
  `~/.config/systemd/user/whatnow.service`. Other systems: not supported, start the server by hand.
- The service runs `whatnow serve <port>`. `./install.sh` restarts it after every new build.
- Logs: `~/.whatnow/server.log`.

## Configuration
`~/.whatnow/config.json` (all keys optional):
```json
{"featuresDir": "/path/to/features", "aliases": {"repo-folder-name": "project-name"}, "port": 7777, "claudeBin": "/path/to/claude"}
```
- `featuresDir` (or `$FEATURES_DIR`): where the features live. Default `~/.whatnow/features`.
- `aliases`: use when a repository folder is not named like the project in `feature.json`.
- `claudeBin`: path to the `claude` executable, for a service whose PATH is short.
- `$WHATNOW_HOME` changes the root (`~/.whatnow`).

## Files
- `skill/SKILL.md`: instructions for Claude: when and how to keep a dashboard, the honesty rules.
- `skill/SCHEMA.md`: the `feature.json` format.
- `skill/template.html` is the feature page, `skill/template-index.html` the home page, `skill/side.js` the bar on the left.
  They are embedded into the program (`assets.go`).
- `app/`: the program. `render.go` validation, derivation and page building; `server.go` the live pages and buttons;
  `hook.go` the hooks; `cli.go` the commands; `transcript.go` reading session transcripts; `main.go` the window.
- `skill/*.py`: the earlier Python version, kept until the Go one has run for a while; `tests/` test it.
- `examples/csv-export.json`: a complete example with made-up data.
- `install.sh`: copies `skill/` into `~/.claude/skills/whatnow`. Edit the files here, then run it again.

## Tests
```sh
go vet ./app
python3 -m unittest discover -s tests
```

## Limitations
- Hooks follow the format in the Claude Code documentation. Some paths (for example resuming a closed session
  from a service) depend on your Claude Code version and macOS permissions.
- Delivery into an open session needs the hooks to be installed and the session to belong to a feature.
- The dashboard shows only what was written into `feature.json` and what the hooks saw. It does not read your code.
