# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

The pages are HTML served by a local Go server. They run in a desktop window (Wails: WebKit on macOS, WebView2 on
Windows, WebKitGTK on Linux) and in any browser at `http://127.0.0.1:<port>/`. The desktop window adds what a browser
cannot: the terminal of a session, system notifications, screenshots, a folder picker.

## Users

One engineer on their own machine who runs several Claude Code sessions in parallel, often across several
repositories, on pieces of work that last days and have several stages. They want one window to look at instead of a
row of terminals and long transcripts: which session works, which waits for them, what each has done, and what is
left. They often step away (a meeting, the night) and come back to catch up. Teams and leads are not the target now.

## Product Purpose

whatnow shows the state of every Claude Code session and of every larger piece of work ("work") in one place, and
lets the user act on it from there: answer a session, write to it, open its real terminal, message many sessions at
once, start new work. Success: the user knows in seconds what needs them, never loses a question a session asked, and
can leave the sessions working without watching them.

## Positioning

The plan of a piece of work (planned versus what really happened, stage by stage) sits next to the live session that
does it, in one window. Everything is read locally from Claude Code's own files and hooks: no cloud, no account, no
telemetry. It shows progress and who has to act, not tokens or costs. It is built for Claude Code only, and goes deep
there (hooks, `claude attach`, transcripts, background sessions) rather than wide across agents.

## Operating Context

- Claude Code sessions run in terminals or in the background (`claude --bg`); whatnow's hooks record what they do,
  and deliver messages from the page when a session's turn ends.
- A work's plan lives in `~/.whatnow/features/<slug>/feature.json`, written by the session itself through the
  `whatnow` skill; `whatnow render` builds the pages.
- Typical moments: a quick glance between other tasks; answering a waiting session; checking git state before a push;
  "I am going to sleep" with the sessions left working overnight; "Good morning" catch-up.
- The window is often kept open beside an editor or terminal for hours.

## Capabilities and Constraints

- Pages: home (Sessions, Work, Favorites), the page of a work (Progress, What is left, Decisions and questions, Git,
  Access), the bar on the left of every page, the floating window of a session (Chat and Terminal).
- Actions: Do it / Write / Not needed on items, free messages to a session, Message sessions (several at once, with a
  real stop after confirmation), New work (a folder or the access of an existing work; same folder or a worktree),
  screenshots for Claude (macOS), alerts, Stay awake.
- Local only: the server listens on 127.0.0.1. The terminal of a session is served only to the app window, never on
  the port. Outward actions (push, deploy, messages to people) are the sessions' business, never the dashboard's.
- Open source and for anyone: no personal paths or defaults of the author in product behaviour.
- The dashboard's own labels are English; the content of a work is written in the language the work is done in.
- User-facing term is "work"; "feature" remains only in file names, code and APIs.
- macOS, Windows and Linux are all supported; some window-only features are macOS-first (screenshots).
- Undecided: workspaces, starting work from a repository URL, screenshots on Linux and Windows.

## Brand Commitments

- The name is lowercase: **whatnow**.
- Voice: plain, short, active sentences in sentence case that say what happens ("Send to 4", "Stop them now",
  "waits for your answer"); no jargon, no apologies, no marketing tone. Buttons keep the same name through a flow.

## Evidence on Hand

- README screenshots in `docs/img/` show a made-up world (projects, sessions, people); no real user data may appear
  in published material.
- There are no testimonials, user counts, benchmarks or case studies; do not invent them.

## Product Principles

1. What needs the user comes first: a waiting question or permission is never hidden behind activity.
2. Show what really happened, with evidence (commits, files, test results), next to what was planned.
3. Nothing leaves the machine, and nothing outward or irreversible happens without the user's explicit word.
4. Act where you look: every state shown has the action next to it.
5. Deep on Claude Code rather than wide across tools.

## Accessibility & Inclusion

No formal standard has been set. Existing commitments: full keyboard use with visible focus, a Motion switch (on /
follow the system / off), and text that stays readable on the dark bar.
