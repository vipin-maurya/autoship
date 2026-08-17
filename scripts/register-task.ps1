<#
.SYNOPSIS
    Registers autoship as a Windows Scheduled Task.

.DESCRIPTION
    autoship is a scheduled one-shot, not a daemon: it runs, decides, possibly
    works, and exits. That is why it costs nothing between ticks, and it is why
    supervision, restart and history are Task Scheduler's job rather than ours.

    The default schedule is every 15 minutes inside a working-hours window
    (spec Q1). The machine is never woken to run it — a release that waits for
    the laptop to be open is the correct trade.

.PARAMETER ExePath
    Path to autoship.exe.

.PARAMETER ConfigPath
    Path to autoship.yaml. Its directory becomes the task's working directory.

.PARAMETER TaskName
    Scheduled task name. Defaults to "autoship".

.PARAMETER IntervalMinutes
    Minutes between ticks. Defaults to 15.

.PARAMETER StartTime
    First run of the day, as HH:mm. Defaults to 09:00.

.PARAMETER WindowHours
    Length of the working-hours window. Defaults to 10 (09:00-19:00).

.PARAMETER DryRun
    Register the task to run `autoship dry-run` instead of `autoship run`.
    Use this for the soak described in docs/scheduling.md.

.PARAMETER WhatIf
    Print the task definition without registering anything.

.EXAMPLE
    powershell -File scripts/register-task.ps1 `
        -ExePath C:\tools\autoship.exe `
        -ConfigPath C:\Users\vm899\repos\ExpenseTracker\autoship.yaml -WhatIf
#>
[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter(Mandatory = $true)][string] $ExePath,
    [Parameter(Mandatory = $true)][string] $ConfigPath,
    [string] $TaskName = 'autoship',
    [int]    $IntervalMinutes = 15,
    [string] $StartTime = '09:00',
    [int]    $WindowHours = 10,
    [switch] $DryRun
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $ExePath)) {
    throw "autoship.exe not found at $ExePath"
}
if (-not (Test-Path -LiteralPath $ConfigPath)) {
    throw "autoship.yaml not found at $ConfigPath"
}

$exe = (Resolve-Path -LiteralPath $ExePath).Path
$config = (Resolve-Path -LiteralPath $ConfigPath).Path
$workingDir = Split-Path -Parent $config
$verb = if ($DryRun) { 'dry-run' } else { 'run' }

$action = New-ScheduledTaskAction `
    -Execute $exe `
    -Argument "$verb --config `"$config`" --quiet" `
    -WorkingDirectory $workingDir

# One daily trigger that repeats every $IntervalMinutes for $WindowHours, so
# the expensive path can only start during hours someone is around to notice it.
$trigger = New-ScheduledTaskTrigger -Daily -At $StartTime
$trigger.Repetition = (New-ScheduledTaskTrigger `
        -Once -At $StartTime `
        -RepetitionInterval (New-TimeSpan -Minutes $IntervalMinutes) `
        -RepetitionDuration (New-TimeSpan -Hours $WindowHours)).Repetition

# A build can outlive the interval; autoship's own lock makes the overlap safe,
# and IgnoreNew keeps Task Scheduler from queueing a backlog behind it.
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -WakeToRun:$false `
    -MultipleInstances IgnoreNew `
    -ExecutionTimeLimit (New-TimeSpan -Hours 2) `
    -RestartCount 0

# Interactive-token principal: the DPAPI secrets are scoped to this user, and
# the release build needs the user's Gradle and Android SDK caches.
$principal = New-ScheduledTaskPrincipal `
    -UserId "$env:USERDOMAIN\$env:USERNAME" `
    -LogonType S4U `
    -RunLevel Limited

$task = New-ScheduledTask `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Principal $principal `
    -Description "autoship: watch $((Split-Path -Leaf $workingDir)) and ship closed-testing releases"

Write-Host "Task name:        $TaskName"
Write-Host "Command:          $exe $verb --config `"$config`" --quiet"
Write-Host "Working dir:      $workingDir"
Write-Host "Schedule:         every $IntervalMinutes min from $StartTime for $WindowHours h, daily"
Write-Host "Run as:           $env:USERDOMAIN\$env:USERNAME (S4U, no stored password)"
Write-Host "Wake to run:      no"
Write-Host "Overlap policy:   IgnoreNew (autoship also holds its own lock)"

if ($PSCmdlet.ShouldProcess($TaskName, 'Register scheduled task')) {
    Register-ScheduledTask -TaskName $TaskName -InputObject $task -Force | Out-Null
    Write-Host ""
    Write-Host "Registered. Inspect it with:  schtasks /query /tn $TaskName /v /fo list"
    Write-Host "Run it once now with:         schtasks /run /tn $TaskName"
}
else {
    Write-Host ""
    Write-Host "-WhatIf: nothing was registered."
}
