<#
.SYNOPSIS
    Compila o instalador de aula para Windows (File Browser Quantum + Tao + Lúa).

.DESCRIPTION
    1. Descarga das Releases os binarios para Windows e comproba o SHA-256 de cada un
       (se non coincide, para: nunca se embebe un binario sen verificar):
         - File Browser Quantum  gtsteffaniak/filebrowser  $FbVersion  (filebrowser.exe)
         - Tao                    piztusistemas/tao         $TaoVersion (Tao-<v>-windows-amd64.exe)
         - Lúa                    piztusistemas/lua         $LuaVersion (lua_<v>_windows_amd64.zip)
    2. Compila cmd/fbservizo (o servizo de Windows que lanza Quantum).
    3. Mete todo en files/ e compila o instalador con `task build` (Wails v3).

    Os hashes de Tao e Lúa saen do .sha256 publicado na propia Release; o de Quantum
    está fixado aquí (é o asset filebrowser.exe da release v1.5.6-stable, a mesma
    versión que instala o playbook de Piztu en Debian).

.EXAMPLE
    .\build-windows.ps1
    .\build-windows.ps1 -TaoVersion 26.11.5 -LuaVersion 26.11.1
#>
[CmdletBinding()]
param(
    [string]$TaoVersion = "26.11.5",
    [string]$LuaVersion = "26.11.1",
    [switch]$SkipFiles
)

$ErrorActionPreference = "Stop"
$FbVersion = "v1.5.6-stable"
$FbSha256  = "a4bbefab9b1570b675344f69e2ec7e6d8c70eb9ecefa3f0132330619be6223c8"

$dir = $PSScriptRoot
$env:PATH = "$env:PATH;$env:USERPROFILE\go\bin"

# Executa un programa externo sen que PowerShell 5.1 trate como fatal o que escriba
# en stderr (go e task escriben alí o progreso). O erro real mírase en $LASTEXITCODE.
function Invoke-Native {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try { & $args[0] @($args | Select-Object -Skip 1) 2>&1 | ForEach-Object { "$_" } }
    finally { $ErrorActionPreference = $prev }
}

function Require-Command([string]$Name, [string]$Hint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Non se atopou '$Name' no PATH. $Hint"
    }
}

function Get-Verified([string]$Url, [string]$Dest, [string]$Sha256) {
    Write-Host "    $Url"
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
    $got = (Get-FileHash -Algorithm SHA256 $Dest).Hash.ToLower()
    if ($got -ne $Sha256.ToLower()) {
        Remove-Item $Dest -Force
        throw "SHA-256 incorrecto en $Url`n  esperado: $Sha256`n  obtido:   $got"
    }
}

# Le o hash do ficheiro .sha256 dunha Release ("<hash>  <nome>" ou só "<hash>").
function Get-PublishedSha([string]$Url) {
    $txt = (Invoke-WebRequest -Uri $Url -UseBasicParsing).Content
    if ($txt -is [byte[]]) { $txt = [Text.Encoding]::UTF8.GetString($txt) }
    return ($txt.Trim() -split '\s+')[0]
}

Write-Host "==> Comprobando ferramentas..." -ForegroundColor Cyan
Require-Command go     "Instala Go: https://go.dev/dl/"
Require-Command wails3 "go install github.com/wailsapp/wails/v3/cmd/wails3@latest"
Require-Command task   "winget install Task.Task"
Require-Command npm    "Instala Node.js: https://nodejs.org/"

$files = Join-Path $dir "files"
if (-not $SkipFiles) {
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("instalador-aula-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        Write-Host "==> File Browser Quantum $FbVersion" -ForegroundColor Cyan
        Get-Verified "https://github.com/gtsteffaniak/filebrowser/releases/download/$FbVersion/filebrowser.exe" `
            (Join-Path $tmp "filebrowser.exe") $FbSha256

        Write-Host "==> Tao $TaoVersion" -ForegroundColor Cyan
        $taoBase = "https://github.com/piztusistemas/tao/releases/download/v$TaoVersion/Tao-$TaoVersion-windows-amd64.exe"
        Get-Verified $taoBase (Join-Path $tmp "tao.exe") (Get-PublishedSha "$taoBase.sha256")

        Write-Host "==> Lúa $LuaVersion" -ForegroundColor Cyan
        $luaBase = "https://github.com/piztusistemas/lua/releases/download/$LuaVersion/lua_${LuaVersion}_windows_amd64.zip"
        $luaZip = Join-Path $tmp "lua.zip"
        Get-Verified $luaBase $luaZip (Get-PublishedSha "$luaBase.sha256")
        $luaDir = Join-Path $tmp "lua"
        Expand-Archive -Path $luaZip -DestinationPath $luaDir -Force
        $luaExe = Get-ChildItem -Path $luaDir -Recurse -Filter "lua.exe" | Select-Object -First 1
        if (-not $luaExe) { throw "o zip de Lúa non leva lua.exe" }

        Write-Host "==> fbservizo (servizo de Windows)" -ForegroundColor Cyan
        Push-Location $dir
        try {
            $env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
            Invoke-Native go build -trimpath -ldflags "-s -w" -o (Join-Path $tmp "fbservizo.exe") ./cmd/fbservizo
            if ($LASTEXITCODE -ne 0) { throw "go build fbservizo fallou" }
        } finally {
            Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
            Pop-Location
        }

        Write-Host "==> Enchendo files\" -ForegroundColor Cyan
        Get-ChildItem $files -Exclude "LEEME.txt" -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
        New-Item -ItemType Directory -Force -Path $files | Out-Null
        Copy-Item (Join-Path $tmp "filebrowser.exe") $files
        Copy-Item (Join-Path $tmp "tao.exe") $files
        Copy-Item $luaExe.FullName (Join-Path $files "lua.exe")
        Copy-Item (Join-Path $tmp "fbservizo.exe") $files
        @"
File Browser Quantum $FbVersion
Tao $TaoVersion
Lúa $LuaVersion
"@ | Set-Content -Encoding utf8 (Join-Path $files "versions.txt")
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
} else {
    Write-Host "==> -SkipFiles: uso files\ tal como está" -ForegroundColor Yellow
}

Push-Location $dir
try {
    Write-Host "==> Tests..." -ForegroundColor Cyan
    Invoke-Native go test ./...
    if ($LASTEXITCODE -ne 0) { throw "os tests fallaron" }

    Write-Host "==> task build (instalador)..." -ForegroundColor Cyan
    Invoke-Native task build
    if ($LASTEXITCODE -ne 0) { throw "task build fallou" }
} finally {
    Pop-Location
}

$exe = Join-Path $dir "build\bin\instalador-aula.exe"
$hash = (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLower()
"$hash  instalador-aula.exe" | Set-Content -Encoding ascii "$exe.sha256"
Write-Host ""
Write-Host "✅ Compilado: $exe" -ForegroundColor Green
Write-Host "   SHA-256: $hash"
