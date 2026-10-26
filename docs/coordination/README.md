# Agent coordination protocol

How Claude, Codex/ChatGPT and any other agent share the D-002 work without doing
the same task twice. The user relays between tools, so everything an agent needs
is a Markdown file in the repository. Nothing here replaces `AGENTS.md`,
`ROADMAP.md` or `TASKS.md`; it covers the gap between them: who works on what
next, and how agents tell each other things.

## Why claims in TASKS.md are not enough

A claim written in `TASKS.md` on a task branch is invisible to every other agent
until that branch is merged. Two agents can claim the same task in the same hour.
So a claim has two parts, and the first one is the one that counts.

1. **The branch is the claim.** A task is taken the moment a branch named for it
   exists on `origin`. Names: `agent/<ID>-<slug>` or `codex/<ID>-<slug>`.
2. `TASKS.md` records owner, branch and worktree for humans, and is updated on
   the task branch as before.

## Starting a task (every agent, every time)

1. `git fetch origin` and read `docs/coordination/DISPATCH.md` on `origin/main`.
2. Check that nobody holds the task:
   `git ls-remote --heads origin '*<ID>*'` must print nothing. Also scan
   `docs/coordination/handoff/` for a note that mentions the task.
3. Take a task from your own lane, in the order of the Next up list. Taking a task
   from another lane needs a handoff note to that lane's owner first.
4. Create the worktree and branch from the latest `main`, and **push the empty
   branch right away** (`git push -u origin <branch>`). That publishes the claim
   before any work happens.
5. Then claim the row in `TASKS.md` as `AGENTS.md` says.

## Lanes

Work is split into lanes by the files each task touches. One owner per lane at a
time. `DISPATCH.md` names the lanes, their owners and their order. A task in
another lane is not taken just because its dependencies are done.

An agent that runs out of lane work says so in a handoff note instead of picking
from another lane.

## Handoff notes

To tell another agent something (a contract changed, a task is finished, a file
is about to be touched, a question), add one new file:

`docs/coordination/handoff/YYYY-MM-DD-<from>-to-<to>-<topic>.md`

One file per note keeps parallel branches from conflicting. Notes are never
edited by anyone else. Say what changed, what the reader must do, and what is
still open. Keep it short. The user may relay a note by pasting it, so it must
make sense alone.

## Merged is the source of truth

`DISPATCH.md` is read from `origin/main`. Only the coordinator (the user, or an
agent they name) edits the lane table and the Next up list, in a small docs-only
change. Agents update their own task row in `TASKS.md` and add handoff notes, and
nothing else in this directory.

## Briefing to paste into a new ChatGPT or Codex session

> Repository: ollisulopuisto/postilaatikko. Before editing, read `AGENTS.md`,
> `ROADMAP.md`, `TASKS.md` and `docs/coordination/DISPATCH.md` from the latest
> `main`, then follow `docs/coordination/README.md`. You work in the lane
> DISPATCH.md assigns to Codex. Take the first unclaimed task in your Next up
> list, confirm with `git ls-remote --heads origin '*<ID>*'` that no branch holds
> it, push your branch before starting, and work in your own worktree. Do not
> merge, do not push to `main`, and do not touch files owned by another active
> task. When done, update your `TASKS.md` row and add a handoff note if anything
> you changed affects another lane.
