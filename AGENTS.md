# Agent instructions for vidpolish

## Code review (lrc / LiveReview)

This repo has LiveReview git hooks installed. By default, `git commit` opens
a **blocking browser review** that waits for a human decision before the
commit can complete — that will hang an agent session.

For a normal change, use one of these instead of a bare `git commit`:

- **Non-interactive review, 1-2 passes for anything non-trivial**: run
  `lrc review --no-serve --output json --staged` after `git add`. Read the
  JSON findings and fix anything real, then re-stage and run it again. Two
  passes is usually enough — don't loop indefinitely chasing diminishing
  findings. Note this command only *reports* findings — it does **not**
  write an attestation, so `git commit` right after it will still trigger
  the blocking browser review. Follow it with an explicit skip or vouch
  (below) once you're satisfied with the findings.
- **Skip review** (only when the user explicitly asks to skip/bypass
  review, or you just ran the non-interactive review above and judged the
  change safe): `lrc review --staged --skip`. This writes an attestation
  for the currently staged tree without running AI review at all.
- **Manual vouch** (user explicitly wants to approve without AI review):
  `lrc review --staged --vouch`.

After a skip or vouch succeeds, run `git commit` as its own command (not
chained with `&&`/`;` onto another command — the hook rejects that). The
attestation is tied to the exact staged tree; it's cleared the moment a
commit happens, so re-staging anything (including `git commit --amend`)
requires re-running the skip/vouch step before the next commit. Never leave
a bare `git commit` to open the blocking browser review and then just wait
on it — that hangs the session; if a plain review or commit does end up
waiting on a browser decision, stop it and re-do the commit via skip/vouch
instead of leaving it pending.

Always run `bash <lrc plugin dir>/scripts/ensure-lrc.sh` first if a review
command reports the backend isn't ready.

## Version bumps

`VERSION` is the single source of truth (plain semver, no `v` prefix).
`make bump-patch` / `make bump-minor` / `make bump-major` bump it in place;
`make tag-release` then commits `VERSION` and tags `vX.Y.Z`.

- **Default to a patch bump** for fixes, small features, and anything that
  doesn't change or add a public API/behavior contract — this is almost
  always what "bump the version" or "make a release" means unless said
  otherwise.
- **Minor or major bumps require asking the user first.** Don't decide
  unilaterally that a change is "feature-worthy" (minor) or
  "breaking" (major) — propose it and get a yes before running
  `make bump-minor` / `make bump-major`.
- After bumping: `git diff VERSION` to sanity check, `make tag-release`,
  then `git push && git push --tags`, then `make release-publish` (needs
  `gh` authenticated) to cross-compile and publish the GitHub Release,
  including the Windows GUI installer (`vidpolish-setup-windows-amd64.exe`).
