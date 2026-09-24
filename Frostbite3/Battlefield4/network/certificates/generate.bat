```bat
@echo off
setlocal EnableExtensions

rem ============================================================
rem BlazeSDK Legacy OTG3 Certificate Generator
rem ============================================================

set "ROOT_DIR=%~dp0"
set "CERT_DIR=%ROOT_DIR%Certificates"
set "OPENSSL=C:\Program Files\OpenSSL-Win64\bin\openssl.exe"

set "CA_NAME=OTG3"
set "C_NAME=gosredirector"
set "MOD_NAME=gosredirector_mod"
set "PASSWORD=123456"

if not exist "%OPENSSL%" (
    echo.
    echo ERROR: OpenSSL was not found:
    echo "%OPENSSL%"
    echo.
    pause
    exit /b 1
)

if not exist "%CERT_DIR%" mkdir "%CERT_DIR%"
cd /d "%CERT_DIR%"

echo.
echo ============================================================
echo BlazeSDK OTG3 Certificate Generator
echo ============================================================
echo Project: %ROOT_DIR%
echo Certs:   %CERT_DIR%
echo OpenSSL: %OPENSSL%
echo ============================================================
echo.

rem ------------------------------------------------------------
rem Save current Windows timezone
rem ------------------------------------------------------------

for /f "tokens=*" %%A in ('tzutil /g') do set "OLD_TZ=%%A"

if not defined OLD_TZ (
    echo ERROR: Could not determine current Windows timezone.
    pause
    exit /b 1
)

echo Current timezone:
echo %OLD_TZ%
echo.

echo Current system time:
powershell -NoProfile -Command "Get-Date; [DateTime]::UtcNow"
echo.

rem ------------------------------------------------------------
rem Switch Windows timezone to UTC.
rem This does NOT change the system clock.
rem It only changes the timezone used by the legacy
rem OpenSSL Windows build when generating certificate dates.
rem ------------------------------------------------------------

echo Switching Windows timezone to UTC for certificate generation...

tzutil /s "UTC"

if errorlevel 1 (
    echo ERROR: Could not switch timezone to UTC.
    pause
    exit /b 1
)

echo.
echo OpenSSL generation timezone:
tzutil /g
echo.

echo Time after timezone switch:
powershell -NoProfile -Command "Get-Date; [DateTime]::UtcNow"
echo.

rem ------------------------------------------------------------
rem Clean old generated files
rem ------------------------------------------------------------

echo Cleaning old certificate files...

del /q "%CA_NAME%.key.pem" 2>nul
del /q "%CA_NAME%.crt" 2>nul
del /q "%CA_NAME%.srl" 2>nul
del /q "%C_NAME%.key.pem" 2>nul
del /q "%C_NAME%.csr" 2>nul
del /q "%C_NAME%.crt" 2>nul
del /q "%C_NAME%.der" 2>nul
del /q "%MOD_NAME%.crt" 2>nul
del /q "%MOD_NAME%.pfx" 2>nul

rem ------------------------------------------------------------
rem Create OTG3 CA private key
rem ------------------------------------------------------------

echo.
echo [1/8] Creating OTG3 CA private key...

"%OPENSSL%" genrsa -aes128 ^
-out "%CA_NAME%.key.pem" ^
-passout pass:%PASSWORD% ^
1024

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Remove CA private-key password
rem ------------------------------------------------------------

echo.
echo [2/8] Removing CA private-key password...

"%OPENSSL%" rsa ^
-in "%CA_NAME%.key.pem" ^
-out "%CA_NAME%.key.pem" ^
-passin pass:%PASSWORD%

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Create OTG3 CA certificate
rem ------------------------------------------------------------

echo.
echo [3/8] Creating OTG3 Certificate Authority...

"%OPENSSL%" req -new -md5 -x509 ^
-days 28124 ^
-key "%CA_NAME%.key.pem" ^
-out "%CA_NAME%.crt" ^
-subj "/OU=Online Technology Group/O=Electronic Arts, Inc./L=Redwood City/ST=California/C=US/CN=OTG3 Certificate Authority"

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Create gosredirector private key
rem ------------------------------------------------------------

echo.
echo [4/8] Creating gosredirector private key...

"%OPENSSL%" genrsa -aes128 ^
-out "%C_NAME%.key.pem" ^
-passout pass:%PASSWORD% ^
1024

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Remove certificate private-key password
rem ------------------------------------------------------------

echo.
echo Removing gosredirector private-key password...

"%OPENSSL%" rsa ^
-in "%C_NAME%.key.pem" ^
-out "%C_NAME%.key.pem" ^
-passin pass:%PASSWORD%

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Create CSR
rem ------------------------------------------------------------

echo.
echo [5/8] Creating certificate signing request...

"%OPENSSL%" req -new ^
-key "%C_NAME%.key.pem" ^
-out "%C_NAME%.csr" ^
-subj "/CN=gosredirector.ea.com/OU=Global Online Studio/O=Electronic Arts, Inc./ST=California/C=US"

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Sign certificate
rem ------------------------------------------------------------

echo.
echo [6/8] Signing gosredirector certificate...

"%OPENSSL%" x509 -req ^
-in "%C_NAME%.csr" ^
-CA "%CA_NAME%.crt" ^
-CAkey "%CA_NAME%.key.pem" ^
-CAcreateserial ^
-out "%C_NAME%.crt" ^
-days 10000 ^
-md5

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Display generated certificate dates
rem ------------------------------------------------------------

echo.
echo ============================================================
echo GENERATED CERTIFICATE
echo ============================================================
echo.

"%OPENSSL%" x509 ^
-in "%C_NAME%.crt" ^
-noout ^
-subject ^
-issuer ^
-serial ^
-dates

if errorlevel 1 goto :error

echo.
echo ============================================================
echo IMPORTANT
echo ============================================================
echo.
echo The certificate above should now use UTC/GMT correctly.
echo.
echo Your Windows UTC time should match the certificate
echo NotBefore time, within a few seconds.
echo.
echo ============================================================

rem ------------------------------------------------------------
rem Export DER
rem ------------------------------------------------------------

echo.
echo [7/8] Exporting certificate to DER...

"%OPENSSL%" x509 ^
-outform der ^
-in "%C_NAME%.crt" ^
-out "%C_NAME%.der"

if errorlevel 1 goto :error

if not exist "%C_NAME%.der" (
    echo ERROR: DER file was not created.
    goto :error
)

echo.
echo DER exported:
echo "%CERT_DIR%\%C_NAME%.der"

rem ------------------------------------------------------------
rem Restore original timezone BEFORE manual patching.
rem ------------------------------------------------------------

echo.
echo Restoring original Windows timezone:

tzutil /s "%OLD_TZ%"

if errorlevel 1 (
    echo WARNING: Could not restore timezone automatically.
    echo Original timezone was:
    echo %OLD_TZ%
) else (
    echo %OLD_TZ%
)

echo.
echo Current time:
powershell -NoProfile -Command "Get-Date; [DateTime]::UtcNow"

echo.
echo ============================================================
echo MANUAL DER PATCH
echo ============================================================
echo.
echo Patch:
echo "%CERT_DIR%\%C_NAME%.der"
echo.
echo Save the patched certificate as:
echo "%CERT_DIR%\%MOD_NAME%.der"
echo.
echo ============================================================
pause

rem ------------------------------------------------------------
rem Check patched DER
rem ------------------------------------------------------------

if not exist "%MOD_NAME%.der" (
    echo.
    echo ERROR: Patched DER was not found:
    echo "%CERT_DIR%\%MOD_NAME%.der"
    echo.
    pause
    exit /b 1
)

rem ------------------------------------------------------------
rem Convert patched DER to CRT
rem ------------------------------------------------------------

echo.
echo [8/8] Converting patched DER to CRT...

"%OPENSSL%" x509 ^
-inform der ^
-in "%MOD_NAME%.der" ^
-out "%MOD_NAME%.crt"

if errorlevel 1 goto :error

if not exist "%MOD_NAME%.crt" (
    echo ERROR: Patched CRT was not created.
    goto :error
)

echo.
echo ============================================================
echo PATCHED CERTIFICATE
echo ============================================================
echo.

"%OPENSSL%" x509 ^
-in "%MOD_NAME%.crt" ^
-noout ^
-subject ^
-issuer ^
-serial ^
-dates

if errorlevel 1 goto :error

rem ------------------------------------------------------------
rem Create PFX
rem ------------------------------------------------------------

echo.
echo Creating PFX...

"%OPENSSL%" pkcs12 -export ^
-out "%MOD_NAME%.pfx" ^
-inkey "%C_NAME%.key.pem" ^
-in "%MOD_NAME%.crt" ^
-certfile "%CA_NAME%.crt" ^
-passout pass:%PASSWORD% ^
-name "%C_NAME%"

if errorlevel 1 goto :error

if not exist "%MOD_NAME%.pfx" (
    echo ERROR: PFX was not created.
    goto :error
)

rem ------------------------------------------------------------
rem Verify PFX
rem ------------------------------------------------------------

echo.
echo ============================================================
echo VERIFYING PFX
echo ============================================================
echo.

"%OPENSSL%" pkcs12 ^
-info ^
-in "%MOD_NAME%.pfx" ^
-passin pass:%PASSWORD% ^
-noout

if errorlevel 1 goto :error

echo.
echo ============================================================
echo SUCCESS
echo ============================================================
echo.
echo Certificate:
echo "%CERT_DIR%\%MOD_NAME%.crt"
echo.
echo PFX:
echo "%CERT_DIR%\%MOD_NAME%.pfx"
echo.
echo PFX password:
echo %PASSWORD%
echo.
echo PFX contains:
echo - gosredirector certificate
echo - OTG3 CA certificate
echo - private key
echo.
echo ============================================================
pause
exit /b 0

:error
echo.
echo ============================================================
echo ERROR
echo ============================================================
echo.
echo Certificate generation failed.
echo.
echo Restoring original timezone:
echo %OLD_TZ%
echo.

tzutil /s "%OLD_TZ%" >nul 2>&1

echo Current timezone:
tzutil /g

echo.
pause
exit /b 1
```
