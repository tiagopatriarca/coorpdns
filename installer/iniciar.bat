@echo off
title CoorpDNS Endpoint Agent - TI Plus
color 0A
set SERVER=https://dns.tiplus.inf.br
set TOKEN=

if exist "%~dp0config.txt" (
    set /p TOKEN=<"%~dp0config.txt"
)

if "%TOKEN%"=="" (
    if not "%1"=="" set TOKEN=%1
)

if "%TOKEN%"=="" (
    echo Digite o token do dispositivo gerado no painel web:
    set /p TOKEN="Token: "
    echo %TOKEN%>"%~dp0config.txt"
)

echo Iniciando CoorpDNS Agent...
"%~dp0coorpdns-agent.exe" -server %SERVER% -token %TOKEN%
pause
