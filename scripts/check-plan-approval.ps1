Param(
	[string]$BaseRef = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-BaseRef {
	Param([string]$Preferred)
	if ($Preferred -ne "") {
		return $Preferred
	}
	$candidates = @("origin/main", "main")
	foreach ($c in $candidates) {
		git rev-parse --verify $c *> $null
		if ($LASTEXITCODE -eq 0) {
			return $c
		}
	}
	return "HEAD~1"
}

function Get-ChangedFiles {
	Param([string]$Base)
	$files = New-Object System.Collections.Generic.HashSet[string]
	$mergeBase = (git merge-base HEAD $Base 2>$null)
	if ($LASTEXITCODE -eq 0 -and $mergeBase) {
		$rangeFiles = git diff --name-only "$mergeBase..HEAD"
		foreach ($f in $rangeFiles) {
			if ($f) { [void]$files.Add($f.Trim()) }
		}
	}
	$workTree = git diff --name-only
	foreach ($f in $workTree) {
		if ($f) { [void]$files.Add($f.Trim()) }
	}
	$staged = git diff --name-only --cached
	foreach ($f in $staged) {
		if ($f) { [void]$files.Add($f.Trim()) }
	}
	return @($files)
}

function Is-CodeFile {
	Param([string]$Path)
	$p = $Path.Replace('\', '/')
	if ($p -match '^PLANS-.*\.md$') { return $false }
	if ($p -in @("AGENTS.md", "AUDIT-TEMPLATE.md", "PLANS-TEMPLATE.md", "README.md")) { return $false }
	if ($p -match '^docs/') { return $false }
	if ($p -match '\.md$') { return $false }
	if ($p -match '\.ya?ml$') { return $false }
	return $true
}

$branch = (git rev-parse --abbrev-ref HEAD).Trim()
if (-not $branch -or $branch -eq "HEAD") {
	Write-Error "Unable to resolve branch name; cannot locate PLANS-{branch}.md"
	exit 2
}

$planPath = "PLANS-$branch.md"
$base = Resolve-BaseRef -Preferred $BaseRef
$changed = Get-ChangedFiles -Base $base
$codeChanged = @($changed | Where-Object { Is-CodeFile $_ })

if ($codeChanged.Count -eq 0) {
	Write-Host "PASS: no code-file changes detected."
	exit 0
}

if (-not (Test-Path $planPath)) {
	Write-Error "FAIL: code-file changes detected but $planPath is missing."
	exit 1
}

$planText = Get-Content -Path $planPath -Raw
if ($planText -notmatch 'Approval:\s+Approved v\d+') {
	Write-Error "FAIL: code-file changes detected but $planPath lacks 'Approval: Approved vN'."
	Write-Host "Changed code files:"
	foreach ($f in $codeChanged) { Write-Host " - $f" }
	exit 1
}

Write-Host "PASS: code-file changes detected and $planPath contains approval token."
exit 0
