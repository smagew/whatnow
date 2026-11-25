---
name: whatnow
description: Keep a visual dashboard for a large feature or project that spans several repos (plan vs actual per stage, percent of the plan done, open items by who has to act). Use when the user starts a big multi-phase feature or project, asks for "whatnow", a feature dashboard or the status of a big feature, or when a phase of a feature that already has a dashboard is finished, a problem is found, or a check is run. Not for small tasks.
---

# whatnow

One `feature.json` per big feature is the source of truth. `whatnow render` validates it and builds a standalone `panel.html` (tabs: Progress, What is left, Decisions and questions). The page answers: what was planned, what really happened, in which order, what is verified, and what blocks the release.

**Language.** The dashboard's own labels are English. Write the *content* of `feature.json` (titles, plan items, facts, notes) in the language the work is done in; do not translate it. Reply to the user in the language they use.

## Files
- The program is `~/.claude/skills/whatnow/whatnow` (one file; it is also the desktop app). Always call it by that full path.
- `~/.claude/skills/whatnow/whatnow render <dir>/feature.json --open` builds one feature page.
- `~/.claude/skills/whatnow/whatnow render --index --open` rebuilds **every** feature page under the features folder plus the home page `index.html` (all features, per project, what needs attention). Use this form after every update.
- `SCHEMA.md`: every field of `feature.json`. Read it before writing or editing one.
- `examples/csv-export.json`: a complete example with made-up data. Copy its shape.

## Live status (hooks and server)
The hook (`whatnow hook`) and the server (`whatnow serve`, or the desktop app) give a page that updates itself and shows which session works where. That part is automatic and does not depend on you: Claude Code runs the hook, the server reads its files. Everything else (stages, blockers) is still written by you into `feature.json`. Start the server with `~/.claude/skills/whatnow/whatnow serve`, or make it start at login with `~/.claude/skills/whatnow/whatnow autostart install` (macOS and Linux). A session is linked to a feature when it works on its files (editing `feature.json` or running `whatnow render` on it counts), so always touch the feature through its `feature.json`; the user can also attach a session by hand on the feature page. Never edit the user's `settings.json` to install the hooks without being asked: `~/.claude/skills/whatnow/whatnow hooks print` shows the entries, `docs/hooks-settings.example.json` in the project has a copy.

## Messages from the page
The feature page (served by the server) has buttons per item: Do it, Ask, Not needed. A message arrives in your session as a system reminder or as extra prompt text that starts with `[whatnow] Message from the dashboard`. It names the feature, stage and item and carries the user's note. Handle it like a normal request:
- **Do it**: do the work. Then record it in `feature.json` (a new fact with evidence, blockers), run `~/.claude/skills/whatnow/whatnow render --index`, and close the message: `~/.claude/skills/whatnow/whatnow inbox resolve <id> "one line of what was done"`.
- **Ask**: answer in the conversation, then resolve with the answer in one line.
- Never edit `overrides.json`: it holds the user's "not needed" decisions. An item the user marked as not needed (kind `n`) is intentionally skipped; do not bring it back.
- If you cannot do it, say why in the conversation and resolve with the reason, so the page does not wait forever.

## Where the files live
Default: `~/.whatnow/features/<slug>/feature.json` (outside the project repos, so nothing leaks into them). The folder is resolved from `$FEATURES_DIR`, then `featuresDir` in `~/.whatnow/config.json`, then the default; `whatnow render --index` and the server both use it. Never write the dashboard into a project repo unless the user asks. If the user names another place, use it. Tell the user once where the file is and that it is not under git.

## Start a feature (`new`)
1. Pick a short slug. Ask nothing if the feature and projects are clear from the conversation.
2. Write the plan **before any code**: stages in the order they should happen, each with `plan` items (what the stage includes), empty `facts`. Deployment and checks are not separate lists: deployment is a stage (or stages) of the plan, and a check you intend to run is a plan item of the stage it belongs to. Put in `decisions` what the user already agreed.
3. Give a stage a `kind` when it clearly is one of: `plan`, `research`, `code`, `test`, `review`, `deploy`, `docs` (the page marks it with an icon). A feature has only the stages its work really needs: do not add a test, review or deployment stage because other features have one.
4. Run `~/.claude/skills/whatnow/whatnow render --index --open` and show it. The feature also appears on the home page.

## The home page
`index.html` is the single place to see all features at once: one card per feature with its projects, the percent of the plan that is done (every stage weighs the same and counts by the share of its items done), the stages as a strip, problems, and how long ago the file was updated. A feature counts as finished when every stage is done and it has no blockers. It shows only what was written into the files, so keep `updated` honest.

## Update a feature (`update`)
Update after every finished phase, every found problem, every check that ran, every commit/push, and when the user reports something. One update = edit `feature.json`, run `~/.claude/skills/whatnow/whatnow render --index`, tell the user what changed in status terms ("stage 4: tests ✓, live database ○"). Also set `updated` to today's date: the home page marks a feature stale after 3 days without an update, so an untouched file looks abandoned.

Rules that keep the dashboard honest:
- **The plan column is not rewritten.** Add to `facts`, do not edit `plan` after work started. New work that was not planned goes into a new stage with `"unplanned": true`. Wrong plan items are flagged with a `bad` fact, not deleted.
- **Stage order = real order.** Add stages in the order they actually happened. Number them in sequence.
- **Never claim verified without evidence.** A fact is `y` only if something was run or read in this session (tests with result, a diff, a command output, the user's confirmation). Otherwise `p` or `q`. State what was NOT verified as its own `q` fact.
- **Record evidence.** Commit hashes, test counts, who confirmed. A fact marked done says what proves it. Write the hash of the commit into the item it belongs to (`commit 3f2a9c1`): the page links the item to the commit on the Git tab and back, and shows whether it is pushed.
- **Say how it ended.** The line of an item is short; its result goes into the detail, the third element of the fact: `["y", "short line", "detail"]`. Write there what you would tell the user in the terminal when you finish: what was done, the important details and numbers, what was checked and how, what did not work, what is left. Several paragraphs and `- ` lists are fine. Do this for every finished item that is more than a trivial step, and give every finished stage a `"result"` in the same form. The user opens a finished step to read how it ended; a one-line item with no detail tells them almost nothing.
- **Problems are facts.** A mistake by an agent or by you, a rework, a wrong earlier claim: a fact with what happened and what fixed it. Use `fixed` once it is solved (it stays in the history and does not turn the stage red); use `bad` only while the problem is still open, and change it to `fixed` when you solve it.
- **A request goes into the plan the moment you take it.** When the user gives you new work on the feature, or changes what was agreed, write it into `feature.json` before you start working and run `whatnow render --index`: every new task as an item of the stage it belongs to (kind `q`; `w` for the one you begin now), or a new stage with `"unplanned": true` when it fits none; a question that needs the user's decision goes into `questions` and into a blocker with `who`. The user watches the dashboard: a request they made must show up there at once, not after the work is done. The page marks what is new since they last looked.
- **Show where you are.** When you start an item that will take more than a few minutes, set its kind to `w` (add it as a `q` fact first if it is not in the list) and run `whatnow render --index`: the Progress tab then shows the stage and the item you are on. Keep at most one `w` in a feature. When you stop working on it, change it to what is true (`y`, `p`, `bad` or back to `q`); never leave a `w` behind at the end of a turn that finished the item.
- **Change the kind, keep the item.** When an item moves on (`q` → `y`), change its kind and extend its text in place instead of deleting it and adding a new one elsewhere: the page records what changed and when (`history.json`, written by `whatnow render`; never edit it), and an item that keeps its place keeps its history.
- **State is derived, not typed.** Stage colour and whether the feature is ready to release are computed by `whatnow render`. Do not try to set them.
- **Do not commit or push** because of a dashboard update. The dashboard is not part of the work in the repos.

## Blockers and next steps
Rewrite `blockers` and `nextSteps` at every update from the current state; do not append blindly.
- Candidates: `bad` and `p` facts of stages already passed, `q` facts that say something was not verified, open `questions`.
- Each blocker needs `why` (the concrete risk), `todo` (one action), `proj`, `stage` (a real stage number) and `who`: `you` if only the user can do it, `look` if the user has to check or decide, `agent` if you can do it now, `later` for later phases. The page groups the open items by `who`, so a missing `who` shows up as "Not sorted yet".
- Severity: **high** = wrong data, security or a failed production release is possible, or unreviewed code could reach production. **mid** = a process gap that stops the release (no merge request, tests not compared). **low** = polish (translations, wording).
- `nextSteps` is a real sequence: each step depends on the previous one. Order by dependency, not importance.
- Never force the answer to "Yes": `whatnow render` shows it only when there are no blockers and every stage is done.

## Checking your work
After rendering, if you changed a template or rendered something new, take a screenshot with headless Chrome and look at it (`"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --window-size=1280,900 --screenshot=out.png file://.../panel.html`). `whatnow render` exits with a clear message when the data breaks a rule; fix the data, do not loosen the rule.
