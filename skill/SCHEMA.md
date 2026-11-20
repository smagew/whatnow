# feature.json

All text is Russian. Text fields may contain `<code>…</code>` and `<b>…</b>`.

```json
{
  "title": "Экспорт отчёта в CSV",
  "sub": "Проекты backend и web.",
  "summary": "One or two sentences: where the feature stands right now.",
  "projects": ["backend", "web"],
  "updated": "2026-01-15",
  "ladder": { "api": [ ... six entries ... ] },
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
Per project, six entries in this order: Написан, Тесты, Код-ревью, Проверен руками, На staging, На production.
Each entry is `"ok" | "part" | "no"` or `["ok", "note"]`. From "Код-ревью" onwards, `ok` needs a note saying what was checked and where.

## stages (in the order they really happened)
```json
{ "n": 4, "title": "…", "verdict": "short result", "proj": "api",
  "unplanned": false,
  "plan":  ["what the stage was supposed to include"],
  "facts": [ ["y","done"], ["bad","problem or rework"], ["p","partly done"], ["q","not done"] ] }
```
`n` is an integer, strictly increasing. Stage colour is derived: all `q` = grey, any `bad` = red, any `p`/`q` = amber, else green. `unplanned: true` marks work that was not in the plan.

## deploy
```json
{ "n": 1, "title": "Staging: api", "sub": "миграция, проверка на базе",
  "items": ["not done yet", {"t": "done item", "s": "y"}] }
```
Item = text (not done) or `{t, s}` with `s` in `y | bad | p | q`. Counts and step state are derived.

## checks
```json
{ "name": "Модульные тесты", "hint": "optional",
  "cells": { "api": ["ok","пройдены","note"], "os": ["na","не нужно",""] } }
```
Cell = `[status, label, note]`, status in `ok | part | no | na`. Every project must have a cell.

## blockers
```json
{ "t": "title", "why": "concrete risk", "todo": "one action",
  "proj": ["api"], "stage": 4, "sev": "high" }
```
`sev` in `high | mid | low`. `stage` must exist.
