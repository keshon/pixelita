@echo off
rem ---------------------------------------------------------------------------
rem  Pixelita build helper.
rem
rem    build          build every command into bin\
rem    build test     run the unit tests
rem    build vet      run Go's static checks
rem    build check    run tests, vet, and build
rem    build clean    remove bin\
rem
rem  Messages are ASCII so cmd.exe does not depend on the active code page.
rem ---------------------------------------------------------------------------
setlocal
cd /d "%~dp0"

set MODE=%~1
if "%MODE%"=="" set MODE=build

where go >nul 2>nul
if errorlevel 1 (
    echo ERROR: go was not found. Install Go and add it to PATH first.
    exit /b 1
)

if /i "%MODE%"=="build" goto :build
if /i "%MODE%"=="test" goto :test
if /i "%MODE%"=="vet" goto :vet
if /i "%MODE%"=="check" goto :check
if /i "%MODE%"=="clean" goto :clean

echo ERROR: unknown mode "%MODE%".
echo Use: build ^| test ^| vet ^| check ^| clean
exit /b 1

rem ---------------------------------------------------------------------------
:build
if not exist bin mkdir bin
echo Building Pixelita commands into bin\ ...
go build -o bin\ ./cmd/...
if errorlevel 1 exit /b 1
echo.
echo Done: bin\
exit /b 0

rem ---------------------------------------------------------------------------
:test
echo Running tests...
go test ./...
exit /b %errorlevel%

rem ---------------------------------------------------------------------------
:vet
echo Running vet...
go vet ./...
exit /b %errorlevel%

rem ---------------------------------------------------------------------------
:check
call :test
if errorlevel 1 exit /b 1
call :vet
if errorlevel 1 exit /b 1
call :build
exit /b %errorlevel%

rem ---------------------------------------------------------------------------
:clean
if not exist bin (
    echo Nothing to clean.
    exit /b 0
)
echo Removing bin\ ...
rmdir /s /q bin
if errorlevel 1 exit /b 1
echo Done.
exit /b 0
