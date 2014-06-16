@echo off

set thisPath=%cd%
set serviceName=RBGClient

.\ServiceTools\instsrv.exe %serviceName% %thisPath%\ServiceTools\srvany.exe

REG ADD HKLM\SYSTEM\CURRENTCONTROLSET\SERVICES\%serviceName% /f
REG ADD HKLM\SYSTEM\CURRENTCONTROLSET\SERVICES\%serviceName%\Parameters /f
REG ADD HKLM\SYSTEM\CURRENTCONTROLSET\SERVICES\%serviceName%\Parameters /f /v AppDirectory /t REG_SZ /d %thisPath%
REG ADD HKLM\SYSTEM\CURRENTCONTROLSET\SERVICES\%serviceName%\Parameters /f /v Application /t REG_SZ /d %thisPath%\client.exe
REG ADD HKLM\SYSTEM\CURRENTCONTROLSET\SERVICES\%serviceName%\Parameters /f /v AppParameters /t REG_SZ /d 1

net start %serviceName%

echo "[Successful]"
pause