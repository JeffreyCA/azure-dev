param(
    [switch] $ShortMode,
    [string] $UnitTestCoverageDir = 'cover-unit',
    [string] $IntegrationTestTimeout = '120m',
    [string] $IntegrationTestCoverageDir = 'cover-int'
)

$ErrorActionPreference = 'Stop'

$gopath = go env GOPATH
if ($LASTEXITCODE) {
    throw "go env GOPATH failed with exit code: $LASTEXITCODE, stdout: $gopath"
}

$gotestsumBinary = "gotestsum"
if ($IsWindows) {
    $gotestsumBinary += ".exe"
}

$gotestsum = Join-Path $gopath "bin" $gotestsumBinary
if (-not (Test-Path $gotestsum)) {
    throw "gotestsum is not installed at $gotestsum"
}

$flakyTestTarget = $env:AZD_TEST_FLAKY_TARGET
$flakyTestRepeat = $env:AZD_TEST_FLAKY_REPEAT
$skipDotnetCacheClear = [string]::Equals($env:AZD_SKIP_DOTNET_CACHE_CLEAR, "true", [StringComparison]::OrdinalIgnoreCase)

function Clear-DotNetCaches {
    if ($skipDotnetCacheClear) {
        Write-Host "Skipping dotnet cache clear (AZD_SKIP_DOTNET_CACHE_CLEAR=true)"
        return
    }

    Write-Host "Clearing dotnet caches"
    & dotnet nuget locals all --clear
    if ($LASTEXITCODE) {
        throw "dotnet nuget locals all --clear failed with exit code: $LASTEXITCODE"
    }

    & dotnet workload clean
    if ($LASTEXITCODE) {
        throw "dotnet workload clean failed with exit code: $LASTEXITCODE"
    }
}

function Write-DotNetDiagnostics {
    Write-Host "dotnet --info"
    & dotnet --info
    Write-Host "dotnet --list-sdks"
    & dotnet --list-sdks
    Write-Host "dotnet --list-runtimes"
    & dotnet --list-runtimes
    Write-Host "dotnet nuget locals all --list"
    & dotnet nuget locals all --list
}

function New-EmptyDirectory {
    param([string]$Path)
    if (Test-Path $Path) {
        Remove-Item -Force -Recurse $Path | Out-Null
    }

    New-Item -ItemType Directory -Force -Path $Path
}

$isFlakyRun = -not [string]::IsNullOrWhiteSpace($flakyTestTarget)
if ($isFlakyRun) {
    $flakyIterationsValue = 1
    if (-not [string]::IsNullOrWhiteSpace($flakyTestRepeat)) {
        if (-not [int]::TryParse($flakyTestRepeat, [ref]$flakyIterationsValue)) {
            Write-Host "Unable to parse AZD_TEST_FLAKY_REPEAT='$flakyTestRepeat'. Defaulting to 1."
            $flakyIterationsValue = 1
        }
    }

    $flakyIterations = [math]::Max(1, $flakyIterationsValue)
    Write-Host "Flaky test mode enabled. Running '$flakyTestTarget' $flakyIterations time(s)."

    Write-DotNetDiagnostics
    Write-Host "Clearing go test cache"
    go clean -testcache

    # Ensure coverage directories exist for artifact upload expectations.
    $null = New-EmptyDirectory -Path $UnitTestCoverageDir
    $flakyCoverRoot = New-EmptyDirectory -Path $IntegrationTestCoverageDir

    $oldGOCOVERDIR = $env:GOCOVERDIR
    $oldGOEXPERIMENT = $env:GOEXPERIMENT

    try {
        for ($i = 1; $i -le $flakyIterations; $i++) {
            Write-Host "Attempt $i of $flakyIterations"
            Clear-DotNetCaches

            $attemptCoverDir = Join-Path $flakyCoverRoot.FullName "run-$i"
            New-Item -ItemType Directory -Force -Path $attemptCoverDir | Out-Null

            # GOCOVERDIR enables any binaries built with '-cover' to write coverage output.
            $env:GOCOVERDIR = $attemptCoverDir
            $env:GOEXPERIMENT = ""

            & $gotestsum -- ./test/functional -v -timeout $IntegrationTestTimeout -run $flakyTestTarget -count=1
            if ($LASTEXITCODE) {
                exit $LASTEXITCODE
            }
        }
    } finally {
        $env:GOCOVERDIR = $oldGOCOVERDIR
        $env:GOEXPERIMENT = $oldGOEXPERIMENT
    }

    exit 0
}

$unitCoverDir = New-EmptyDirectory -Path $UnitTestCoverageDir
Write-Host "Running unit tests..."

Write-Host "Clearing go test cache"
go clean -testcache

# --test.gocoverdir is currently a "under-the-cover" way to pass the coverage directory to a test binary
# See https://github.com/golang/go/issues/51430#issuecomment-1344711300
#
# As of Go 1.25, it’s still an “under-the-hood” option.
& $gotestsum -- ./... -short -v -cover -args --test.gocoverdir="$($unitCoverDir.FullName)"
if ($LASTEXITCODE) {
    exit $LASTEXITCODE
}

if ($ShortMode) {
    Write-Host "Short mode, skipping integration tests"
    exit 0
}

Write-Host "Running integration tests..."
Write-DotNetDiagnostics
$intCoverDir = New-EmptyDirectory -Path $IntegrationTestCoverageDir

$oldGOCOVERDIR = $env:GOCOVERDIR
$oldGOEXPERIMENT = $env:GOEXPERIMENT

# GOCOVERDIR enables any binaries (in this case, azd.exe) built with '-cover',
# to write out coverage output to the specific directory.
$env:GOCOVERDIR = $intCoverDir.FullName
# Set any experiment flags that are needed for the tests.
$env:GOEXPERIMENT=""

try {
    & $gotestsum -- ./... -v -timeout $IntegrationTestTimeout
    if ($LASTEXITCODE) {
        exit $LASTEXITCODE
    }
} finally {
    $env:GOCOVERDIR = $oldGOCOVERDIR
    $env:GOEXPERIMENT = $oldGOEXPERIMENT
}