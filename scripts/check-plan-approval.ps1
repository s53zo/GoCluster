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
	if ($p -in @("AGENTS.md", "AUDIT-TEMPLATE.md", "README.md")) { return $false }
	if ($p -match '^docs/') { return $false }
	if ($p -match '\.md$') { return $false }
	if ($p -match '\.ya?ml$') { return $false }
	return $true
}

$base = Resolve-BaseRef -Preferred $BaseRef
$changed = Get-ChangedFiles -Base $base
$codeChanged = @($changed | Where-Object { Is-CodeFile $_ })

if ($codeChanged.Count -eq 0) {
	Write-Host "PASS: no code-file changes detected."
	exit 0
}

if (-not (Test-Path "AGENTS.md")) {
	Write-Error "FAIL: code-file changes detected but AGENTS.md is missing."
	exit 1
}

$agentsText = Get-Content -Path "AGENTS.md" -Raw
$requiredPatterns = @(
	'Scope Ledger',
	'Approval handshake',
	'Approved vN'
)

foreach ($pattern in $requiredPatterns) {
	if ($agentsText -notmatch [regex]::Escape($pattern)) {
		Write-Error "FAIL: code-file changes detected but AGENTS.md is missing required workflow phrase '$pattern'."
		Write-Host "Changed code files:"
		foreach ($f in $codeChanged) { Write-Host " - $f" }
		exit 1
	}
}

Write-Host "PASS: code-file changes detected and AGENTS.md contains scope-ledger approval workflow guardrails."
exit 0
