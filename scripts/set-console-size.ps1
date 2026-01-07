<#
.SYNOPSIS
  Resize the PowerShell window to cover the ANSI console minimum (90×68).

.DESCRIPTION
  Many terminals default to too-small dimensions for the fixed ANSI UI. This helper
  sets both the buffer and window size so `ui.mode: ansi` has enough columns/rows.
  It is safe to rerun; it only grows the buffer before changing the viewport.

.PARAMETER Width
  Target width in columns (default: 90).

.PARAMETER Height
  Target height in rows (default: 68).

.EXAMPLE
  .\scripts\set-console-size.ps1
  Ensures VS Code/PowerShell is at least 90×68.

.EXAMPLE
  .\scripts\set-console-size.ps1 -Width 150 -Height 74
  Grow the console to the larger dimensions before launching the exe.
#>

param (
    [int]$Width = 90,
    [int]$Height = 68
)

try {
    $rawUI = $Host.UI.RawUI
} catch {
    Write-Error "Unable to access RawUI: $_"
    exit 1
}

$buffer = $rawUI.BufferSize
$window = $rawUI.WindowSize

# grow buffer first; PowerShell will throw if window is larger than buffer
if ($buffer.Width -lt $Width) {
    $buffer.Width = $Width
}
if ($buffer.Height -lt $Height) {
    $buffer.Height = $Height
}
$rawUI.BufferSize = $buffer

$targetWidth = [Math]::Min($Width, $buffer.Width)
$targetHeight = [Math]::Min($Height, $buffer.Height)

$newWindow = New-Object System.Management.Automation.Host.Size
$newWindow.Width = $targetWidth
$newWindow.Height = $targetHeight

try {
    $rawUI.WindowSize = $newWindow
    Write-Host "Console resized to $($rawUI.WindowSize.Width)x$($rawUI.WindowSize.Height)."
} catch {
    Write-Warning "Unable to resize window: $_"
}
