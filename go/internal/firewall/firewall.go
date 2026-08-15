// Package firewall opens port 22, makes the SSH/Tailscale services persistent,
// and installs self-healing scheduled tasks.
package firewall

import (
	"os"

	"smol-tailscaler/internal/ui"
	"smol-tailscaler/internal/winutil"
)

const ensureScriptPath = `C:\ProgramData\Tailscaler\ensure.ps1`

// Ensure applies the firewall rule, service persistence, and self-healing.
func Ensure() error {
	ensureRule()
	ensureServicePersists("sshd")
	ensureServicePersists("Tailscale")
	ensureSelfHealingTasks()
	return nil
}

// ensureRule allows inbound SSH on port 22, creating the rule if the check
// (or the rule itself) is missing.
func ensureRule() {
	if winutil.RunCmdOK("netsh", "advfirewall", "firewall", "show", "rule", "name=OpenSSH-Server-In-TCP") != nil {
		if err := winutil.RunCmdOK("netsh", "advfirewall", "firewall", "add", "rule",
			"name=OpenSSH-Server-In-TCP", "dir=in", "action=allow", "protocol=TCP", "localport=22"); err != nil {
			ui.Warn("firewall rule: %s", err)
		} else {
			ui.Ok("Firewall rule created for port 22")
		}
	} else {
		ui.Ok("Firewall rule already exists")
	}
}

// ensureServicePersists makes a service boot with Windows and survive crashes.
// Delayed start gives the network stack time to come up (sshd fails to bind
// port 22 at boot otherwise), and failure actions restart it if it dies later.
func ensureServicePersists(name string) {
	if !winutil.ServiceExists(name) {
		ui.Warn("%s service not found - skipping", name)
		return
	}
	winutil.RunCmdOK("sc.exe", "config", name, "start=delayed-auto")
	winutil.RunCmdOK("sc.exe", "failure", name, "reset=86400", "actions=restart/5000/restart/10000")
	if !winutil.ServiceRunning(name) {
		if err := winutil.RunCmdOK("sc.exe", "start", name); err != nil && !winutil.ServiceRunning(name) {
			ui.Warn("could not start %s: %s", name, err)
		}
	}
	ui.Ok(name + " set to auto-start (delayed) with restart-on-failure")
}

// ensureSelfHealingTasks registers SYSTEM scheduled tasks that re-apply the
// service start types, start sshd/Tailscale with retries, force-connect the
// tailnet, and recreate the firewall rule if it vanished. Runs at boot, logon,
// and daily, so a failed start, crashed service, or removed rule can't leave
// the box unreachable until a manual re-run.
func ensureSelfHealingTasks() {
	writeEnsureScript()
	tr := "powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File " + ensureScriptPath
	winutil.RunCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH", "/tr", tr,
		"/sc", "onstart", "/ru", "SYSTEM", "/rl", "highest", "/f")
	winutil.RunCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH Logon", "/tr", tr,
		"/sc", "onlogon", "/ru", "SYSTEM", "/rl", "highest", "/f")
	winutil.RunCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH Daily", "/tr", tr,
		"/sc", "daily", "/st", "00:00", "/ru", "SYSTEM", "/rl", "highest", "/f")
	ui.Ok("Self-healing scheduled tasks installed (boot + logon + daily)")
}

// writeEnsureScript drops the self-healing script to disk. It retries service
// starts for up to ~2 minutes (network stack may not be ready at boot) and
// force-connects Tailscale with `tailscale up --unattended`.
func writeEnsureScript() {
	const ps = `$ErrorActionPreference = 'SilentlyContinue'
sc.exe config sshd start=delayed-auto
sc.exe config Tailscale start=delayed-auto
foreach ($svc in @('sshd', 'Tailscale')) {
  for ($i = 0; $i -lt 12; $i++) {
    Start-Service $svc -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 10
    if ((Get-Service $svc).Status -eq 'Running') { break }
  }
}
$ts = 'C:\Program Files\Tailscale\tailscale.exe'
if (Test-Path $ts) { & $ts up --unattended }
netsh advfirewall firewall show rule name=OpenSSH-Server-In-TCP > $null 2>&1
if ($LASTEXITCODE -ne 0) {
  netsh advfirewall firewall add rule name=OpenSSH-Server-In-TCP dir=in action=allow protocol=TCP localport=22
}
`
	_ = os.MkdirAll(`C:\ProgramData\Tailscaler`, 0)
	_ = os.WriteFile(ensureScriptPath, []byte(ps), 0)
}
