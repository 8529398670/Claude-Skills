@echo off
rem Set up the venv and run the server from this checkout, outside Docker. The
rem Windows twin of run.sh: same commands, same behavior, except where noted.
rem
rem   run.bat                    create/update .venv if needed, stop the copy
rem                              this checkout started last, start a fresh one
rem   run.bat manage <command>   run an admin command against it (see below)
rem   run.bat <anything else>    same: passed straight to `python -m app`
rem
rem The server runs in the foreground: its log streams here and Ctrl-C stops it.
rem Running run.bat again from another window stops this one and takes over,
rem so "edit, run.bat, reload the page" is the whole loop.
rem
rem With arguments it is a one-off instead -- the same venv, then `python -m
rem app` with those arguments -- and the running server is left alone. `manage`
rem opens the database directly, which SQLite's WAL mode makes safe alongside
rem the live server.
rem
rem   run.bat manage list-users
rem   run.bat manage reissue-login --user-id 1    fresh login link (locked out)
rem   run.bat manage create-login --name "Grace" --role user
rem   run.bat manage create-invite --user-id 1 --label "Book club"
rem   run.bat manage create-key --user-id 1 --name "CI"
rem   run.bat manage paths                        where the state lives
rem   run.bat config check
rem   run.bat version
rem
rem Where it differs from run.sh, Windows forces it:
rem
rem   - There is no pid file. The old server is whatever process is running
rem     this checkout's .venv\Scripts\python.exe with exactly "-m app", which a
rem     full path pins down, so a stale pid can never point at something else.
rem   - Windows cannot send SIGTERM to another console's process, so the old
rem     server is killed rather than drained. SQLite's WAL survives that.
rem   - requirements.txt is reinstalled when its contents differ from the copy
rem     saved after the last install, rather than when it is newer: batch has
rem     no way to compare file times.
rem
rem This is for development and internal boxes. It is a plain process with your
rem permissions -- none of dockerRun.sh's read-only filesystem, dropped
rem capabilities, or pid and memory limits apply -- so use Docker for anything
rem long-lived or internet-facing.
setlocal
cd /d "%~dp0"

set "VENV=%CD%\.venv"
set "PYTHON=%VENV%\Scripts\python.exe"
rem The requirements.txt that was last installed successfully. Any edit to the
rem real one makes them differ, which is the signal to install again.
set "STAMP=%VENV%\.requirements.stamp"

rem The venv's own python is called by path rather than activating the venv:
rem it finds its packages either way, and nothing here depends on PATH.
rem
rem All of this happens before anything is stopped, so a failed install leaves
rem the server that was already running untouched.
rem
rem A venv whose creation was interrupted has a python but no pip, and would
rem fail every install from then on; --clear rebuilds it from scratch.
set "CREATE="
if not exist "%PYTHON%" set "CREATE=1"
if not defined CREATE if not exist "%STAMP%" "%PYTHON%" -m pip --version >nul 2>&1 || set "CREATE=1"
if not defined CREATE goto venv_ready

rem The py launcher first: on a fresh Windows, `python` on PATH is often the
rem Microsoft Store stub, which opens the Store instead of running anything.
set "BASE_PYTHON="
where py >nul 2>&1 && set "BASE_PYTHON=py -3"
if not defined BASE_PYTHON where python >nul 2>&1 && set "BASE_PYTHON=python"
if defined BASE_PYTHON goto create_venv
echo No Python found. Install Python 3 from https://www.python.org/downloads/
echo (it comes with the py launcher) and run run.bat again.
exit /b 1

:create_venv
echo Creating .venv...
%BASE_PYTHON% -m venv --clear "%VENV%"
if errorlevel 1 exit /b 1

:venv_ready
fc /b requirements.txt "%STAMP%" >nul 2>&1
if not errorlevel 1 goto requirements_ready
echo Installing requirements.txt into .venv...
"%PYTHON%" -m pip install --quiet --upgrade pip
if errorlevel 1 exit /b 1
"%PYTHON%" -m pip install --quiet -r requirements.txt
if errorlevel 1 exit /b 1
copy /y requirements.txt "%STAMP%" >nul

:requirements_ready
if "%~1"=="" goto server
"%PYTHON%" -m app %*
exit /b %errorlevel%

:server
rem First run on this checkout: config.yaml is gitignored, so a fresh clone has
rem none. Without one here the app would quietly create
rem %USERPROFILE%\.config\{{PROJECT_SLUG}}\config.yaml instead, which is not the
rem file this checkout's dockerRun.sh reads. Write it here and stop, like
rem dockerRun.sh, so it gets looked at before the first start.
if exist config.yaml goto stop_old
"%PYTHON%" -m app config init --file .\config.yaml
if errorlevel 1 exit /b 1
echo.
echo For plain http://localhost set server.secure_cookies: false in config.yaml,
echo or the browser drops the session cookie and login silently does nothing.
echo Then run run.bat again.
exit /b 0

:stop_old
rem Stop the copy this checkout started last. .venv\Scripts\python.exe is a
rem small launcher that starts the real interpreter as its child, so the whole
rem tree goes (taskkill /T), and both are waited for before the port is reused.
rem The path goes to PowerShell through the environment rather than inside the
rem command, so no character in it (a quote in a user name, say) can break the
rem quoting.
set "RUN_PYTHON=%PYTHON%"
powershell -NoProfile -Command "$all = @(Get-CimInstance Win32_Process); $old = @($all | Where-Object { $_.ExecutablePath -eq $env:RUN_PYTHON -and $_.CommandLine -match '\s-m\s+app\s*$' }); $ids = @($old.ProcessId) + @($all | Where-Object { $old.ProcessId -contains $_.ParentProcessId } | ForEach-Object { $_.ProcessId }); foreach ($p in $old) { Write-Host ('Stopping the previous server (pid ' + $p.ProcessId + ')...'); taskkill.exe /T /F /PID $p.ProcessId | Out-Null }; foreach ($id in $ids) { Wait-Process -Id $id -Timeout 15 -ErrorAction SilentlyContinue }"

echo Starting {{PROJECT_SLUG}} from %CD%
echo   config   .\config.yaml
echo   state    %USERPROFILE%\.config\{{PROJECT_SLUG}}\ unless server.app_dir says otherwise
echo            (run.bat manage paths)
echo   code     Python changes need run.bat again; static\ edits only a reload
echo.
echo On a fresh database the first-run login link is printed below. Locked out
echo later? run.bat manage reissue-login --user-id 1 from another window.
echo.

"%PYTHON%" -m app
