---
name: whatnow
description: Keep a visual control panel for a large feature or project that spans several repos (plan vs actual per stage, deployment plan vs fact, readiness ladder per project, quality-check matrix, blockers). Use when the user starts a big multi-phase feature or project, asks for "whatnow" / a "feature panel" / "панель функции" / status of a big feature, or when a phase of a feature that already has a panel is finished, a problem is found, or a check is run. Not for small tasks.
---

# whatnow

One `feature.json` per big feature is the source of truth. `render.py` validates it and builds a standalone `panel.html` (tabs: Обзор, Ход работы, Выкладка, Качество, Решения и вопросы). The panel answers: what was planned, what really happened, in which order, what is verified, and what blocks the release.

The panel and all texts in it are in **Russian**. Reply to the user in Cyrillic Russian, short.

## Files
- `render.py`: `python3 ~/.claude/skills/whatnow/render.py <dir>/feature.json --open` builds one panel.
- `render.py --index`: `python3 ~/.claude/skills/whatnow/render.py --index --open` rebuilds **every** panel under the features folder plus the portfolio page `features/index.html` (all features by project, what needs attention today). Use this form after every update.
- `SCHEMA.md`: every field of `feature.json`. Read it before writing or editing one.
- `examples/csv-export.json`: a complete example with made-up data. Copy its shape.

## Where the files live
Default: `~/www/features/<slug>/feature.json` (outside the project repos, so nothing leaks into them). `render.py --index` reads that folder; set `FEATURES_DIR` or pass a path to use another. Never write the panel into a project repo unless the user asks. If the user names another place, use it. Tell the user once where the file is and that it is not under git.

## Start a panel (`new`)
1. Pick a short slug. Ask nothing if the feature and projects are clear from the conversation.
2. Write the plan **before any code**: stages in the order they should happen, each with `plan` items (what the stage includes), empty `facts`, plus the `deploy` steps and the `checks` you intend to run. Put in `decisions` what the user already agreed.
3. Set every ladder cell to `no` and every check cell to `no` (or `na` if truly not applicable).
4. Run `render.py --index --open` and show it. The feature also appears on the portfolio page.

## The portfolio page
`features/index.html` is the single place to see all features at once: per project, what is in progress, % readiness (ladder: ok = 1, part = 0.5, no = 0), how far each project got, problems, and how long ago the file was updated. A feature counts as finished when its ladder is all `ok` and it has no blockers. It shows only what was written into the files, so keep `updated` honest.

## Update a panel (`update`)
Update after every finished phase, every found problem, every check that ran, every commit/push, and when the user reports something. One update = edit `feature.json`, run `render.py --index`, tell the user what changed in status terms ("этап 4: тесты ✓, живая база ○"). Also set `updated` to today's date: the portfolio page marks a feature stale after 3 days without an update, so an untouched file looks abandoned.

Rules that keep the panel honest:
- **The plan column is not rewritten.** Add to `facts`, do not edit `plan` after work started. New work that was not planned goes into a new stage with `"unplanned": true`. Wrong plan items are flagged with a `bad` fact, not deleted.
- **Stage order = real order.** Add stages in the order they actually happened. Number them in sequence.
- **Never claim verified without evidence.** A fact is `y` only if something was run or read in this session (tests with result, a diff, a command output, the user's confirmation). Otherwise `p` or `q`. State what was NOT verified as its own `q` fact.
- **Record evidence.** Commit hashes, test counts, who confirmed. Ladder cells from "Код-ревью" onwards need a note when `ok`; `render.py` refuses an `ok` without one.
- **Problems are facts.** A mistake by an agent or by you, a rework, a wrong earlier claim: a `bad` fact with what happened and what fixed it.
- **State is derived, not typed.** Stage colour, deploy counts and the "Можно выпускать?" answer are computed by `render.py`. Do not try to set them.
- **Do not commit or push** because of a panel update. The panel is not part of the work in the repos.

## Blockers and next steps
Rewrite `blockers` and `nextSteps` at every update from the current state; do not append blindly.
- Candidates: ladder cells that are not `ok` below the highest rung reached, `checks` that are `no`, `q` facts that say something was not verified, open `questions`.
- Each blocker needs `why` (the concrete risk), `todo` (one action), `proj` and `stage` (a real stage number).
- Severity: **high** = wrong data, security or a failed production release is possible, or unreviewed code could reach production. **mid** = a process gap that stops the release (no merge request, tests not compared). **low** = polish (translations, wording).
- `nextSteps` is a real sequence: each step depends on the previous one. Order by dependency, not importance.
- Never force the answer to "Да": `render.py` shows it only when there are no blockers and every ladder cell is `ok`.

## Checking your work
After rendering, if you changed the template or rendered something new, take a screenshot with headless Chrome and look at it (`"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --window-size=1280,900 --screenshot=out.png file://.../panel.html`). `render.py` exits with a clear message when the data breaks a rule; fix the data, do not loosen the rule.
