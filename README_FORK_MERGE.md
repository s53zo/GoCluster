# Fork Merge Workflow (Upstream -> Fork)

This repo is a fork of https://github.com/N2WQ/GoCluster.git.
Local `main` tracks `origin/main` (my fork).
Upstream is configured as `upstream`.

## One-time setup (verify/remediate)
1) Verify remotes:
   - `git remote -v`
2) Ensure upstream exists (only if missing):
   - `git remote add upstream https://github.com/N2WQ/GoCluster.git`
3) Ensure local `main` tracks fork `origin/main`:
   - `git branch --set-upstream-to=origin/main main`

## Regular sync process (keep my changes + merge upstream)
1) Make sure working tree is clean (commit or stash local work):
   - `git status -sb`
2) Fetch upstream changes:
   - `git fetch upstream`
3) Merge upstream into my fork `main`:
   - `git checkout main`
   - `git merge upstream/main`
4) Resolve conflicts if any:
   - edit files
   - `git add -A`
   - `git commit`  (only if merge produced conflicts)
5) Push merged `main` to my fork:
   - `git push origin main`

## If local changes block a merge
1) Stash local changes:
   - `git stash push -m "wip before upstream merge"`
2) Merge upstream:
   - `git merge upstream/main`
3) Re-apply local changes:
   - `git stash pop`
4) Resolve any conflicts, then commit and push:
   - `git add -A`
   - `git commit -m "Sync local changes after upstream merge"`
   - `git push origin main`

## Sanity checks
- Check ahead/behind vs fork:
  - `git status -sb`
- Check upstream delta (optional):
  - `git fetch upstream`
  - `git log --oneline origin/main..upstream/main`

## Conflict resolution checklist
1) Identify conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`) and resolve them.
2) Rebuild/retarget any renamed files if needed.
3) Run a quick compile or targeted tests if you changed logic:
   - `go test ./...` (adjust as needed)
4) Finish the merge:
   - `git add -A`
   - `git commit`
   - `git push origin main`

## Emergency reset (discard local commits to match upstream)
Use only if you are OK losing local commits on `main`.
This rewrites history on your fork.
1) Fetch upstream:
   - `git fetch upstream`
2) Hard reset `main` to upstream:
   - `git checkout main`
   - `git reset --hard upstream/main`
3) Force push to your fork:
   - `git push --force origin main`
