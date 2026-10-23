@echo off
rem Build and run the server from this checkout, outside Docker. The Windows
rem twin of run.sh: same commands, same behavior, except where noted below.
rem
rem   run.bat                    rebuild if anything changed, stop the copy this
rem                              checkout started last, start a fresh one here
rem   run.bat manage <command>   run an admin command against it (see below)
rem   run.bat <anything else>    same: passed straight to the binary
rem
rem The server runs in the foreground: its log streams here and Ctrl-C stops it.
rem Running run.bat again from another window stops this one and takes over,
rem so "edit, run.bat, reload the page" is the whole loop.
rem
rem With arguments it is a one-off instead -- the same build, then the binary
rem with those arguments -- and the running server is left alone. That is what
rem makes the admin CLI usable while it is up: `manage` talks to the live server
rem over its control socket rather than opening the database itself.
rem
rem   run.bat manage list-users
rem   run.bat manage reissue-login -user-id 1     fresh login link (locked out)
rem   run.bat manage create-login -name "Grace" -role user
rem   run.bat manage create-invite -user-id 1 -label "Book club"
rem   run.bat manage create-key -user-id 1 -name "CI"
rem   run.bat manage paths                        where the state lives
rem   run.bat config check
rem   run.bat version
rem
rem Where it differs from run.sh, Windows forces it:
rem
rem   - A running .exe cannot be overwritten. The build goes to .run\ and is
rem     copied over {{PROJECT_SLUG}}.exe only once the old server has exited;
rem     one-off runs use the copy in .run\ and never touch the server's.
rem   - There is no pid file. The old server is whatever process is running
rem     this checkout's {{PROJECT_SLUG}}.exe, which a full path pins down
rem     exactly, so a stale pid can never point at something else.
rem   - Windows cannot send SIGTERM to another console's process, so the old
rem     server is killed rather than drained. bolt is crash-safe, and the
rem     control socket a kill leaves behind is cleared on the next start.
rem
rem This is for development and internal boxes. It is a plain process with your
rem permissions -- none of dockerRun.sh's read-only filesystem, dropped
rem capabilities, or pid and memory limits apply -- so use Docker for anything
rem long-lived or internet-facing.
setlocal
cd /d "%~dp0"

rem >>> template demo -- scaffold.py removes this block from every project.
rem This is the skill's template, not a project: its {{PLACEHOLDERS}} are not
rem filled in, so it cannot run as it is. Stamp a demo project from it instead
rem (..\..\scripts\demo.py, kept in a cache directory with its config and
rem database) and hand over to that project's own run.bat.
if not exist "..\..\scripts\demo.py" goto not_template
set "DEMO_PYTHON=python"
where py >nul 2>&1 && set "DEMO_PYTHON=py -3"
set "DEMO_DIR="
for /f "delims=" %%d in ('%DEMO_PYTHON% ..\..\scripts\demo.py') do set "DEMO_DIR=%%d"
if not defined DEMO_DIR exit /b 1
call "%DEMO_DIR%\run.bat" %*
exit /b %errorlevel%
:not_template
rem <<< template demo

set "BINARY=%CD%\{{PROJECT_SLUG}}.exe"
set "RUN_DIR=%CD%\.run"
set "BUILT=%RUN_DIR%\{{PROJECT_SLUG}}.exe"

if not exist "%RUN_DIR%" mkdir "%RUN_DIR%"

rem Rebuild every time and let Go decide whether that means anything: its build
rem cache makes an unchanged tree a near-instant no-op, and catches every input
rem that matters -- .go files, go.mod, and the embedded static/ and
rem language.yaml -- with no timestamp bookkeeping here to get wrong.
rem
rem This happens before anything is stopped, so a build error leaves the server
rem that was already running untouched.
rem
rem Version metadata is stamped the way build.sh does it, so `run.bat version`
rem says what is running. No -s -w: this build is for debugging, not shipping.
rem The version is the UTC build time (2026.10.23-1412), like run.sh.
set "VERSION=dev"
set "COMMIT=unknown"
for /f "delims=" %%v in ('powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy.MM.dd-HHmm')" 2^>nul') do set "VERSION=%%v"
for /f "delims=" %%c in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%c"
go build -trimpath -ldflags "-X main.version=%VERSION% -X main.commit=%COMMIT%" -o "%BUILT%" .
if errorlevel 1 exit /b 1

if "%~1"=="" goto server
"%BUILT%" %*
exit /b %errorlevel%

:server
rem First run on this checkout: config.yaml is gitignored, so a fresh clone has
rem none. Without one here the binary would quietly create
rem %USERPROFILE%\.config\{{PROJECT_SLUG}}\config.yaml instead, which is the
rem right thing for a portable binary and the wrong one for a checkout. Write it
rem here and stop, like dockerRun.sh, so it gets looked at before the first start.
if exist config.yaml goto stop_old
"%BUILT%" config init -file .\config.yaml
if errorlevel 1 exit /b 1
echo.
echo For plain http://localhost set server.secure_cookies: false in config.yaml,
echo or the browser drops the session cookie and login silently does nothing.
echo Then run run.bat again.
exit /b 0

:stop_old
rem Stop the copy this checkout started last: any process whose image is this
rem checkout's {{PROJECT_SLUG}}.exe. The path goes to PowerShell through the
rem environment rather than inside the command, so no character in it (a
rem quote in a user name, say) can break the quoting.
set "RUN_EXE=%BINARY%"
powershell -NoProfile -Command "$old = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $env:RUN_EXE }); foreach ($p in $old) { Write-Host ('Stopping the previous server (pid ' + $p.ProcessId + ')...'); Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }; foreach ($p in $old) { Wait-Process -Id $p.ProcessId -Timeout 15 -ErrorAction SilentlyContinue }"

rem Not a parenthesized block: a ")" in the checkout's path would end it early.
copy /y "%BUILT%" "%BINARY%" >nul
if not errorlevel 1 goto start
echo Could not replace %BINARY% -- is something else still running it?
exit /b 1

:start

echo Starting {{PROJECT_SLUG}} %VERSION% from %CD%
echo   config   .\config.yaml
echo   state    %USERPROFILE%\.config\{{PROJECT_SLUG}}\ unless server.app_dir says otherwise
echo            (run.bat manage paths)
echo   static   .\static is served from disk, so frontend edits need only a reload
echo.
echo On a fresh database the first-run login link is printed below. Locked out
echo later? run.bat manage reissue-login -user-id 1 from another window.
echo.

"%BINARY%"
