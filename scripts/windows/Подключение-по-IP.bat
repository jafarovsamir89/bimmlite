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

set /p "TARGET_IP=Введите IP машины (например 169.254.x.x): "
if "%TARGET_IP%"=="" (
  echo [ERROR] IP не задан.
  pause
  exit /b 1
)

set /p "SERIES=Введите серию (F или G): "
set "SERIES=%SERIES: =%"
for %%A in (%SERIES%) do set "SERIES=%%~A"
set "SERIES=%SERIES:~0,1%"
if /I "%SERIES%"=="F" (
  set "BRIDGE_PROTOCOL=hsfz"
) else if /I "%SERIES%"=="G" (
  set "BRIDGE_PROTOCOL=doip"
) else (
  echo [ERROR] Допустимы только F или G.
  pause
  exit /b 1
)

set "LOG=%USERPROFILE%\Desktop\bimmlite-connect.txt"
set "BM_EXE=%EXE%"
set "BM_LOG=%LOG%"
set "BRIDGE_TARGET_HOST=%TARGET_IP%"

if not defined BRIDGE_WS_URL set "BRIDGE_WS_URL=wss://34.44.19.28/ws/bridge"
if not defined BRIDGE_TLS_INSECURE set "BRIDGE_TLS_INSECURE=true"

echo [INFO] Серия: %SERIES%
echo [INFO] Протокол: %BRIDGE_PROTOCOL%
echo [INFO] IP машины: %BRIDGE_TARGET_HOST%
echo [INFO] Файл лога: "%LOG%"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference = 'Continue';" ^
  "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::UTF8;" ^
  "Write-Host ('[INFO] Connecting to ' + $env:BRIDGE_TARGET_HOST + ' via ' + $env:BRIDGE_PROTOCOL);" ^
  "& $env:BM_EXE --bridge 2>&1 | Tee-Object -FilePath $env:BM_LOG; exit $LASTEXITCODE"

set "RC=%ERRORLEVEL%"
if not exist "%LOG%" (
  >"%LOG%" echo Подключение завершилось, но лог не был создан.
)

start "" notepad.exe "%LOG%"
pause
exit /b %RC%
