# feature.json

The content of this file (titles, plan items, facts, notes) is written in the language the work is done in.
The dashboard's own labels are English. Text fields may contain `<code>…</code>` and `<b>…</b>`.

```json
{
  "title": "CSV export of reports",
  "sub": "Projects: backend and web.",
  "summary": "One or two sentences: where the feature stands right now.",
  "projects": ["backend", "web"],
  "branches": ["hotfix/export"],
  "updated": "2026-01-15",
  "ladder": { "backend": [ ... six entries ... ] },
  "stages": [ ... ],
  "deploy": [ ... ],
  "rollback": ["text"],
  "checks": [ ... ],
  "decisions": ["text"],
  "questions": ["text"],
  "blockers": [ ... ],
  "nextSteps": ["text"]
}
```

## ladder
Per project, six entries in this order: Written, Tests, Code review, Verified by hand, Staging, Production.
Each entry is `"ok" | "part" | "no"` or `["ok", "note"]`. From "Code review" onwards, `ok` needs a note saying what was checked and where.

## stages (in the order they really happened)
```json
{ "n": 4, "title": "…", "verdict": "short result", "proj": "backend",
  "unplanned": false,
  "plan":  ["what the stage was supposed to include"],
  "facts": [ ["y","done"], ["bad","problem or rework"], ["p","partly done"], ["q","not done"] ] }
```
`n` is an integer, strictly increasing. Stage colour is derived: all `q` = grey, any `bad` = red, any `p`/`q` = amber, else green. `unplanned: true` marks work that was not in the plan.

## deploy
```json
{ "n": 1, "title": "Staging: backend", "sub": "migration, check on the database",
  "items": ["not done yet", {"t": "done item", "s": "y"}] }
```
Item = text (not done) or `{t, s}` with `s` in `y | bad | p | q`. Counts and step state are derived.

## checks
```json
{ "name": "Unit tests", "hint": "optional",
  "cells": { "backend": ["ok", "passed", "note"], "web": ["na", "not needed", ""] } }
```
Cell = `[status, label, note]`, status in `ok | part | no | na`. Every project must have a cell.

## blockers
```json
{ "t": "title", "why": "concrete risk", "todo": "one action",
  "proj": ["backend"], "stage": 4, "sev": "high" }
```
`sev` in `high | mid | low`. `stage` must exist.

## branches (optional)
Branches that belong to the feature. Only needed when the branch name does not contain the feature's folder name
(a branch like `feature/<folder-name>` links a session to its feature automatically).

## overrides.json (written by the server, never by Claude)
Lives next to `feature.json`. It holds the user's "not needed" decisions from the buttons on the feature page:
`{"<item id>": {"kind": "skip", "reason": "...", "ts": 1795180343217}}`. The item id is computed from the stage number
and the item text (`common.fact_id`), so a decision survives reordering but is lost when the text is rewritten.
A finished item (`y`) cannot be marked as not needed.
