[CmdletBinding(DefaultParameterSetName = 'Pid')]
param(
    [Parameter(Mandatory = $true)] [string] $McpExe,
    [Parameter(Mandatory = $true)] [string] $Rbz,
    [Parameter(Mandatory = $true, ParameterSetName = 'Pid')] [Alias('Pid')] [int] $SketchUpPid,
    [Parameter(Mandatory = $true, ParameterSetName = 'Session')] [string] $Session,
    [Parameter(Mandatory = $true)] [string] $SourceModel,
    [Parameter(Mandatory = $true)] [string] $Template,
    [Parameter(Mandatory = $true)] [string] $OutputDir,
    [Parameter(Mandatory = $true)] [string] $Fixture,
    [Parameter(Mandatory = $true)] [string] $WorkflowRunId,
    [Parameter(Mandatory = $true)] [string] $ArtifactId,
    [timespan] $ToolTimeout = [timespan]::FromSeconds(45),
    [timespan] $Timeout = [timespan]::FromMinutes(8)
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    $arguments = @(
        'run', './cmd/live-layout-e2e',
        '-mcp-exe', (Resolve-Path $McpExe).Path,
        '-rbz', (Resolve-Path $Rbz).Path,
        '-source-model', (Resolve-Path $SourceModel).Path,
        '-template', (Resolve-Path $Template).Path,
        '-output-dir', [IO.Path]::GetFullPath($OutputDir),
        '-fixture', (Resolve-Path $Fixture).Path,
        '-workflow-run-id', $WorkflowRunId,
        '-artifact-id', $ArtifactId
    )
    $arguments += @('-tool-timeout', ('{0}s' -f [int] $ToolTimeout.TotalSeconds))
    $arguments += @('-timeout', ('{0}s' -f [int] $Timeout.TotalSeconds))
    if ($PSCmdlet.ParameterSetName -eq 'Pid') {
        $arguments += @('-pid', $SketchUpPid)
    }
    else {
        $arguments += @('-session', $Session)
    }

    & go @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "live-layout-e2e failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
