@echo off
setlocal EnableExtensions DisableDelayedExpansion
chcp 65001 >nul

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..") do set "ROOT=%%~fI"

set "EXE=%ROOT%\bridge\bin\BimmLite.exe"
if not exist "%EXE%" set "EXE=%ROOT%\bin\BimmLite.exe"
if not exist "%EXE%" (
  echo [ERROR] BimmLite.exe не найден.
  echo [ERROR] Ожидался в "%ROOT%\bridge\bin" или "%ROOT%\bin".
  pause
  exit /b 1
)

set "LOG=%USERPROFILE%\Desktop\bimmlite-diagnose.txt"
set "BM_EXE=%EXE%"
set "BM_LOG=%LOG%"

if not defined BRIDGE_WS_URL set "BRIDGE_WS_URL=wss://34.44.19.28/ws/bridge"
if not defined BRIDGE_TLS_INSECURE set "BRIDGE_TLS_INSECURE=true"

echo [INFO] Запускаю диагностику...
echo [INFO] Файл лога: "%LOG%"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference = 'Continue';" ^
  "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::UTF8;" ^
  "Write-Host '[INFO] Diagnose mode started';" ^
  "& $env:BM_EXE --diagnose 2>&1 | Tee-Object -FilePath $env:BM_LOG; exit $LASTEXITCODE"

set "RC=%ERRORLEVEL%"
if not exist "%LOG%" (
  >"%LOG%" echo Диагностика завершилась, но лог не был создан.
)

start "" notepad.exe "%LOG%"
powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('Готово, пришлите файл bimmlite-diagnose.txt','BimmLite') | Out-Null"
pause
exit /b %RC%
