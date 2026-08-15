<#
.SYNOPSIS
    Script 100% Automatizado: Configuración de OpenSSH + Auto-login en Tailscale
.DESCRIPTION
    Crea usuario administrador (default: admin), configura SSH (llave pública manual),
    conecta a Tailscale y valida todos los servicios.
.PARAMETER UserPassword
    Contraseña para el usuario administrador.
.PARAMETER TsAuthKey
    Auth Key de Tailscale (tskey-auth-...).
.PARAMETER UserName
    Nombre del usuario administrador (opcional, default: admin).
.EXAMPLE
    .\setup-ssh.ps1 -UserPassword "MiPassword123" -TsAuthKey "tskey-auth-..."
#>

param(
    [Parameter(Mandatory=$true)]
    [string]$UserPassword,

    [Parameter(Mandatory=$true)]
    [string]$TsAuthKey,

    [string]$UserName = "admin"
)

$ErrorActionPreference = "Stop"
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$ProgressPreference = 'SilentlyContinue'

# ==========================================
# CONFIGURACIÓN
# ==========================================
$targetUser     = $UserName
$sshdConfigPath = "C:\ProgramData\ssh\sshd_config"

# Detectar idioma del sistema para nombres de grupos
function Get-AdminGroupName {
    $adminSids = Get-LocalGroup | Where-Object { $_.SID -like "S-1-5-32-544" }
    if ($adminSids) {
        return $adminSids.Name
    }
    # Fallback: probar nombres comunes
    $commonNames = @("Administrators", "Administradores", "Administratoren", "Administrateurs")
    foreach ($name in $commonNames) {
        try {
            $group = Get-LocalGroup -Name $name -ErrorAction Stop
            return $group.Name
        } catch { continue }
    }
    throw "No se pudo encontrar el grupo de Administradores en este sistema."
}

$adminGroupName = Get-AdminGroupName

# Verificar privilegios de administrador
function Test-Admin {
    $identity  = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Admin)) {
    Write-Host "Error: Este script debe ejecutarse como Administrador." -ForegroundColor Red
    Write-Host "Abre PowerShell como Administrador e intenta de nuevo." -ForegroundColor Red
    exit 1
}

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host " Setup Automático: SSH + Tailscale        " -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "Grupo de administradores detectado: '$adminGroupName'" -ForegroundColor Gray

# 1. Asegurar la instalación de OpenSSH Server
Write-Host "`n[1/6] Verificando OpenSSH Server..." -ForegroundColor Yellow
$sshdService = Get-Service -Name sshd -ErrorAction SilentlyContinue
if ($sshdService) {
    Write-Host "OpenSSH Server ya está instalado (servicio sshd encontrado)." -ForegroundColor Green
} else {
    $sshService = Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*'
    if ($sshService -and $sshService.State -ne 'Installed') {
        Write-Host "Intentando instalar OpenSSH Server vía Windows Update..." -ForegroundColor Gray
        try {
            Add-WindowsCapability -Online -Name $sshService.Name | Out-Null
            Write-Host "OpenSSH Server instalado." -ForegroundColor Green
        } catch {
            Write-Host "Windows Update falló. Instalando desde GitHub..." -ForegroundColor Gray
            try {
                $arch = if ([Environment]::Is64BitOperatingSystem) { "Win64" } else { "Win32" }
                $releaseUrl = "https://api.github.com/repos/PowerShell/Win32-OpenSSH/releases/latest"
                $release = Invoke-RestMethod -Uri $releaseUrl -UseBasicParsing
                $asset = $release.assets | Where-Object { $_.name -match $arch -and $_.name -match "\.zip$" } | Select-Object -First 1
                if (-not $asset) { throw "No se encontró el asset para $arch" }

                $zipPath = "$env:TEMP\openssh.zip"
                Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zipPath -UseBasicParsing

                $installDir = "C:\Program Files\OpenSSH"
                if (-not (Test-Path $installDir)) { New-Item -ItemType Directory -Path $installDir -Force | Out-Null }
                Expand-Archive -Path $zipPath -DestinationPath $installDir -Force
                Remove-Item $zipPath -Force

                Push-Location $installDir
                powershell -ExecutionPolicy Bypass -File .\install-sshd.ps1
                Pop-Location

                Set-Service -Name sshd -StartupType Automatic
                Start-Service -Name sshd
                Write-Host "OpenSSH Server instalado desde GitHub." -ForegroundColor Green
            } catch {
                Write-Host "Warning: No se pudo instalar OpenSSH Server automáticamente (error: $($_.Exception.Message))." -ForegroundColor Yellow
                Write-Host "Instálalo manualmente: Configuración > Aplicaciones > Características opcionales > OpenSSH Server." -ForegroundColor Yellow
            }
        }
    } else {
        Write-Host "OpenSSH Server ya está instalado." -ForegroundColor Green
    }
}

# 2. Crear usuario administrador con acceso total
Write-Host "`n[2/6] Creando usuario administrador '$targetUser'..." -ForegroundColor Yellow
$existingUser = Get-LocalUser -Name $targetUser -ErrorAction SilentlyContinue
if (-not $existingUser) {
    $securePassword = ConvertTo-SecureString $UserPassword -AsPlainText -Force
    New-LocalUser -Name $targetUser -Password $securePassword -Description "Administrador con acceso total" -PasswordNeverExpires | Out-Null
    Add-LocalGroupMember -Group $adminGroupName -Member $targetUser
    Write-Host "Usuario '$targetUser' creado y añadido a '$adminGroupName'." -ForegroundColor Green
} else {
    $securePassword = ConvertTo-SecureString $UserPassword -AsPlainText -Force
    $existingUser | Set-LocalUser -Password $securePassword
    if (-not (Get-LocalGroupMember -Group $adminGroupName -Member $targetUser -ErrorAction SilentlyContinue)) {
        Add-LocalGroupMember -Group $adminGroupName -Member $targetUser
    }
    Write-Host "Usuario '$targetUser' ya existe. Contraseña actualizada." -ForegroundColor Green
}

# 3. Aplicar configuración estándar limpia a sshd_config
Write-Host "`n[3/6] Escribiendo sshd_config..." -ForegroundColor Yellow
$configContent = @"
Port 22
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
PasswordAuthentication yes
PermitEmptyPasswords no
Subsystem sftp sftp-server.exe
"@
Set-Content -Path $sshdConfigPath -Value $configContent

# 4. Instalar Tailscale si no está presente
Write-Host "`n[4/6] Verificando Tailscale..." -ForegroundColor Yellow

function Get-TailscalePath {
    $knownPaths = @(
        "C:\Program Files\Tailscale\tailscale.exe",
        "C:\Program Files (x86)\Tailscale\tailscale.exe"
    )
    foreach ($p in $knownPaths) {
        if (Test-Path $p) { return $p }
    }
    $found = Get-Command tailscale -ErrorAction SilentlyContinue
    if ($found) { return $found.Source }
    return $null
}

$tsPath = Get-TailscalePath

if (-not $tsPath) {
    Write-Host "Tailscale no encontrado. Descargando e instalando..." -ForegroundColor Gray

    $arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
    $tsUrl = "https://pkgs.tailscale.com/stable/tailscale-setup-latest-$arch.msi"
    $tsInstaller = "$env:TEMP\tailscale-installer.msi"

    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $tsUrl -OutFile $tsInstaller -UseBasicParsing

    Write-Host "Instalando Tailscale..." -ForegroundColor Gray
    Start-Process msiexec.exe -ArgumentList "/i `"$tsInstaller`" /quiet /norestart" -Wait -NoNewWindow
    Remove-Item $tsInstaller -Force

    $tsPath = Get-TailscalePath
    if (-not $tsPath) {
        throw "Tailscale se instaló pero no se encontró el ejecutable."
    }
    Write-Host "Tailscale instalado exitosamente en: $tsPath" -ForegroundColor Green
} else {
    Write-Host "Tailscale encontrado en: $tsPath" -ForegroundColor Green
}

# 5. Autenticación Automática en Tailscale
Write-Host "`n[5/6] Conectando a Tailscale mediante Auth Key..." -ForegroundColor Yellow
& $tsPath status 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    & $tsPath up --auth-key=$TsAuthKey --unattended
    Write-Host "Tailscale vinculado a la red y configurado en modo unattended." -ForegroundColor Green
} else {
    & $tsPath up --unattended
    Write-Host "Tailscale ya conectado. Modo unattended confirmado." -ForegroundColor Green
}

# 6. Configurar firewall y servicios
Write-Host "`n[6/6] Configurando firewall y servicios..." -ForegroundColor Yellow
if (-not (Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -DisplayName "OpenSSH Server (sshd)" -Direction Inbound -Protocol TCP -LocalPort 22 -Action Allow | Out-Null
    Write-Host "Regla de firewall para SSH creada." -ForegroundColor Green
} else {
    Write-Host "Regla de firewall para SSH ya existe." -ForegroundColor Green
}

if (Get-Service -Name sshd -ErrorAction SilentlyContinue) {
    Set-Service -Name "sshd" -StartupType Automatic
    Restart-Service -Name "sshd" -Force
    Write-Host "sshd configurado para inicio automático." -ForegroundColor Green
} else {
    Write-Host "Warning: Servicio sshd no encontrado. Instala OpenSSH Server manualmente." -ForegroundColor Yellow
}

if (Get-Service -Name "Tailscale" -ErrorAction SilentlyContinue) {
    Set-Service -Name "Tailscale" -StartupType Automatic
    Start-Service -Name "Tailscale"
}

# Reporte Final
try {
    $tsStatus = & $tsPath status --json 2>$null
    $tsIP = $null
    if ($tsStatus) {
        try {
            $tsInfo = $tsStatus | ConvertFrom-Json
            $tsIP = $tsInfo.Self.Addresses | Select-Object -First 1
        } catch {}
    }
} catch {}
if (-not $tsIP) {
    $tsIP = "<ejecutar 'tailscale ip -4' para obtener>"
}

$hostname = $env:COMPUTERNAME

Write-Host "`n" -NoNewline
Write-Host " ==========================================" -ForegroundColor Cyan
Write-Host " DESPLIEGE COMPLETADO - GUARDA ESTA INFO   " -ForegroundColor Cyan
Write-Host " ==========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host " Usuario:       " -NoNewline -ForegroundColor Yellow
Write-Host "$targetUser" -ForegroundColor White
Write-Host " Contraseña:    " -NoNewline -ForegroundColor Yellow
Write-Host "$UserPassword" -ForegroundColor White
Write-Host " Hostname:      " -NoNewline -ForegroundColor Yellow
Write-Host "$hostname" -ForegroundColor White
Write-Host " Tailscale IP:  " -NoNewline -ForegroundColor Yellow
Write-Host "$tsIP" -ForegroundColor White
Write-Host " Tailscale CLI: " -NoNewline -ForegroundColor Yellow
Write-Host "$tsPath" -ForegroundColor White
Write-Host ""
Write-Host " ------------------------------------------" -ForegroundColor Cyan
Write-Host " Conectar con:" -ForegroundColor Cyan
Write-Host "   ssh $targetUser@$tsIP" -ForegroundColor White
Write-Host " ------------------------------------------" -ForegroundColor Cyan
Write-Host ""

$services = @("Tailscale")
if (Get-Service -Name sshd -ErrorAction SilentlyContinue) { $services += "sshd" }
Get-Service -Name $services -ErrorAction SilentlyContinue | Select-Object Name, Status, StartType | Format-Table -AutoSize

Write-Host " Usuarios locales:" -ForegroundColor Cyan
$users = Get-LocalUser | Sort-Object Name
$users | ForEach-Object {
    $hasPassword = if ($_.PasswordLastSet) { "Si" } else { "No" }
    $role = if ($_.SID -like "S-1-5-32-544-*" -or (Get-LocalGroupMember -Group $adminGroupName -Member $_.Name -ErrorAction SilentlyContinue)) { "Admin" } else { "User" }
    $status = if ($_.Enabled) { "Activo" } else { "Desactivado" }
    Write-Host "   $($_.Name)" -NoNewline -ForegroundColor White
    Write-Host " | $status" -NoNewline -ForegroundColor Gray
    Write-Host " | $role" -NoNewline -ForegroundColor Gray
    Write-Host " | Password: $hasPassword" -ForegroundColor Gray
}

Write-Host "`n¡Equipo accesible remotamente. Guarda la contraseña antes de cerrar esta ventana!" -ForegroundColor Green
