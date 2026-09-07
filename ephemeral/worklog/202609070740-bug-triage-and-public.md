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
