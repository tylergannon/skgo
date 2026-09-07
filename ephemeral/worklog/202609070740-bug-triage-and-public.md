# Bug triage, three tractor missions, repo made public

Right after `gh repo edit --visibility public`, the very next `git pull`
returned "remote: Your repository is disabled" (HTTP 403). The API showed
`disabled: false` seconds later and the pull succeeded on retry. It is a
transient on GitHub's side, not a flag on the repo: retry once before
assuming the flip broke anything, and expect a tractor agent pushing in that
window to see the same error.

Triage order for the open issues, and why: #45 and #44 were both hit porting
a real app and block adoption; #38 is a visible regression from the SSR
slice; #39 shares document assembly with #38 so it waits for that merge;
#41 is five separate gaps and needs splitting; #42 is measure-first perf;
#40 is parked by its own text ("decide once a real app hits it").

Missions dispatched from ephemeral/tractor/missions/ (copies of the mission
template with the goal filled in). Ownership lines in each goal are what keep
the three parallel runs from colliding: transport encoding (#45), generate /
adapter / manifest (#44), document assembly for deferred values (#38).

## A run that finished without opening its PR looks exactly like one that did

The #38 run exited 0 with three commits, tests, screenshots and a worklog on
its branch, and no PR. Its worktree also held one untracked screenshot. Tractor
reports COMPLETED either way, so "the PR exists" has to be checked, not
inferred from the exit. The #44 and #45 runs from the same template did open
theirs, so the template is fine; the check belongs in the manager's routine
after every run, before the validator is dispatched.

A branch cut before a sibling PR merged carries that PR's inverse in
`git diff main`. Diff against `git merge-base main <branch>` to see what the
branch actually did.

## The mission template's own warning was not enough

The first #39 run read AGENTS.md, mapped kit correctly, created its worktree,
dispatched a background Opus builder "per Delegate missions, not methods", and
returned in under four minutes to say the run was still in flight. Its exit
killed the builder; the worktree had zero commits. The template's comment
about this sits above the YAML where the agent never sees it, and the goal
text said only "do not return while any subagent you dispatched is still
running", which the agent read as a rule about reporting, not about exiting.
The relaunched mission says it plainly: build in this turn; if you delegate,
block. Watch for the same shape on any run that exits fast with a worktree
and no commits.

The failed run's kit map was worth keeping: for a remote form post kit leaves
`form:` null and carries the submission in `<global>.data.f`, so the issue
text's "form: instead of always null" was wrong. Recorded on #39, with the
instruction to verify it from source.

## End of the morning: four merged, what the validators were worth

PRs #47, #55, #54 and #58 merged, all four with an independent Opus
validation first. Two of the four had real defects a green suite and a
plausible PR body had hidden (#54: an untested code path and a wrong-output
bug; #58: four kit-fidelity gaps). The validation shape that worked: break
the code, require the test to fail; rerun both suites; open every screenshot;
compare the wire against kit's lines. The manager never read a diff.

Every rebase in the morning conflicted only on tracked screenshots (29 the
first time, one the second). That is issue #56's cost, measured.

Dev-mode scope: the e2e dev suite needs a Go proxy started by hand alongside
`just dev`, because Playwright's BASE_URL always targets Go. No recipe does
this. It is item 10 on the board's polish list.
