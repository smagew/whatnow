# feature.json

The content of this file (titles, plan items, facts, notes) is written in the language the work is done in.
The dashboard's own labels are English. Text fields may contain `<code>…</code>` and `<b>…</b>`; any other markup is shown as plain text, so `Map<K, V>` can be written as is.

```json
{
  "title": "CSV export of reports",
  "sub": "Projects: backend and web.",
  "summary": "One or two sentences: where the feature stands right now.",
  "projects": ["backend", "web"],
  "updated": "2026-01-15",
  "stages": [ ... ],
  "decisions": ["text"],
  "questions": ["text"],
  "blockers": [ ... ],
  "nextSteps": ["text"]
}
```

## stages (in the order they really happened)
```json
{ "n": 4, "title": "…", "verdict": "short result", "proj": "backend", "kind": "test",
  "unplanned": false,
  "plan":  ["what the stage was supposed to include"],
  "facts": [ ["y","done"], ["fixed","problem found and already fixed"], ["bad","open problem"], ["p","partly done"], ["q","not done"], ["w","being worked on right now"] ] }
```
`n` is an integer, strictly increasing. `kind` is optional: `plan | research | code | test | review | deploy | docs`; the page marks the stage with an icon (without it the title is read for a hint). The percent of a feature is the share of its plan that is done: every stage weighs the same and counts by the share of its items done (`p` = half, "not needed" is left out). Stage colour is derived and says how worried to be: all `q` = grey, any `bad` = red, everything done = green, 80% or more done = light green ("almost"), otherwise amber. `fixed` counts as done: the mistake stays visible in the list but does not colour the stage. Use `bad` only for a problem that is still open; change it to `fixed` once it is solved. `unplanned: true` marks work that was not in the plan.

`w` marks the one item being worked on right now. The Progress tab shows that stage as the current one, with the item named on it. It counts as not done. Without a `w` item the current stage is the furthest stage that was started.

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

## history.json (written by whatnow render and the server, never by Claude)
Lives next to `feature.json`. Every render compares the items with the previous render and records what changed and when:
`{"ts": 1795180343217, "fid": "<item id>", "stage": 3, "was": "q", "now": "y", "t": "item text"}` (`was` is `null` for a new item).
The time is the modification time of `feature.json`. The Progress tab uses it to show what changed recently. An item whose text
is rewritten in place (same stage, same position) keeps its history; an item that is moved and rewritten at once counts as new.
The first render of a feature only takes a snapshot.
