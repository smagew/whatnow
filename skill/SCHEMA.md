# feature.json

The content of this file (titles, plan items, facts, notes) is written in the language the work is done in.
The dashboard's own labels are English. Text fields may contain `<code>…</code>` and `<b>…</b>`.

```json
{
  "title": "CSV export of reports",
  "sub": "Projects: backend and web.",
  "summary": "One or two sentences: where the feature stands right now.",
  "projects": ["backend", "web"],
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
  "facts": [ ["y","done"], ["fixed","problem found and already fixed"], ["bad","open problem"], ["p","partly done"], ["q","not done"] ] }
```
`n` is an integer, strictly increasing. Stage colour is derived and says how worried to be: all `q` = grey, any `bad` = red, everything done = green, 80% or more done = light green ("almost"), otherwise amber. `fixed` counts as done: the mistake stays visible in the list but does not colour the stage. Use `bad` only for a problem that is still open; change it to `fixed` once it is solved. `unplanned: true` marks work that was not in the plan.

## deploy
```json
{ "n": 1, "title": "Staging: backend", "sub": "migration, check on the database",
  "items": ["not done yet", {"t": "done item", "s": "y"}] }
```
Item = text (not done) or `{t, s}` with `s` in `y | bad | p | q` (`fixed` is for stage facts only). Counts and step state are derived.

## checks
```json
{ "name": "Unit tests", "hint": "optional",
  "cells": { "backend": ["ok", "passed", "note"], "web": ["na", "not needed", ""] } }
```
Cell = `[status, label, note]`, status in `ok | part | no | na`. Every project must have a cell.

## blockers
```json
{ "t": "title", "why": "concrete risk", "todo": "one action",
  "proj": ["backend"], "stage": 4, "sev": "high", "who": "you" }
```
`sev` in `high | mid | low`. `stage` must exist. `who` says who has to act, and the dashboard groups the open items by it:
`you` (only the user can do it), `look` (the user has to check or decide), `agent` (the agent can do it now), `later` (a later phase).
An item without `who` lands in a "Not sorted yet" group, so always set it.

## overrides.json (written by the server, never by Claude)
Lives next to `feature.json`. It holds the user's "not needed" decisions from the buttons on the feature page:
`{"<item id>": {"kind": "skip", "reason": "...", "ts": 1795180343217}}`. The item id is computed from the stage number
and the item text (`common.fact_id`), so a decision survives reordering but is lost when the text is rewritten.
A finished item (`y`) cannot be marked as not needed.
