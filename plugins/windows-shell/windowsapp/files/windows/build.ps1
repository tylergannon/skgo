param(
    [ValidateSet('Build', 'Run', 'Package')][string]$Action = 'Build',
    [ValidateSet('Debug', 'Release')][string]$Configuration = 'Debug',
    [string]$Certificate
)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if (!(Test-Path 'native\bin\skgo_native_core_shared.dll')) { throw 'Run skgo plugin windows-shell prepare on your development machine and transfer windows/ including SDK/ and native/ first.' }
    if ($Action -eq 'Package') {
        if (!$Certificate) { throw 'Package requires -Certificate with your development or distribution signing certificate.' }
        New-Item -ItemType Directory -Force build | Out-Null
        winapp package SKGoShell.csproj --arch x64 -c $Configuration --self-contained --cert $Certificate --output build\SKGoShell.msix -p WindowsPackageType=MSIX -p EnableMsixTooling=true
        if ($LASTEXITCODE -ne 0) { throw 'Windows packaging failed' }
    } else {
        dotnet build SKGoShell.csproj -c $Configuration -r win-x64 -p:Platform=x64
        if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
        if ($Action -eq 'Run') {
            $targetDir = dotnet msbuild SKGoShell.csproj -nologo -p:Configuration=$Configuration -p:RuntimeIdentifier=win-x64 -p:Platform=x64 -getProperty:TargetDir
            if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve Windows output directory' }
            $process = Start-Process -FilePath (Join-Path ($targetDir | Select-Object -Last 1) 'SKGoShell.exe') -PassThru
            Write-Output ('Started SKGo Shell, PID ' + $process.Id)
        }
    }
} finally { Pop-Location }
