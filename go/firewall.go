package main

import (
	"os"
)

const ensureScriptPath = `C:\ProgramData\Tailscaler\ensure.ps1`

func stepFirewallAndServices() {
	step(6, 6, "Firewall + services")

	ensureFirewallRule()
	ensureServicePersists("sshd")
	ensureServicePersists("Tailscale")
	ensureSelfHealingTasks()
}

// ensureFirewallRule allows inbound SSH on port 22, creating the rule if the
// check (or the rule itself) is missing.
func ensureFirewallRule() {
	if runCmdOK("netsh", "advfirewall", "firewall", "show", "rule", "name=OpenSSH-Server-In-TCP") != nil {
		if err := runCmdOK("netsh", "advfirewall", "firewall", "add", "rule",
			"name=OpenSSH-Server-In-TCP", "dir=in", "action=allow", "protocol=TCP", "localport=22"); err != nil {
			warn("firewall rule: %s", err)
		} else {
			ok("Firewall rule created for port 22")
		}
	} else {
		ok("Firewall rule already exists")
	}
}

// ensureServicePersists makes a service boot with Windows and survive crashes.
// Delayed start gives the network stack time to come up (sshd fails to bind
// port 22 at boot otherwise), and failure actions restart it if it dies later.
func ensureServicePersists(name string) {
	if !serviceExists(name) {
		warn("%s service not found - skipping", name)
		return
	}
	runCmdOK("sc.exe", "config", name, "start=delayed-auto")
	runCmdOK("sc.exe", "failure", name, "reset=86400", "actions=restart/5000/restart/10000")
	if !serviceRunning(name) {
		if err := runCmdOK("sc.exe", "start", name); err != nil && !serviceRunning(name) {
			warn("could not start %s: %s", name, err)
		}
	}
	ok(name + " set to auto-start (delayed) with restart-on-failure")
}

// ensureSelfHealingTasks registers SYSTEM scheduled tasks that re-apply the
// service start types, start sshd/Tailscale with retries, force-connect the
// tailnet, and recreate the firewall rule if it vanished. Runs at boot, logon,
// and daily, so a failed start, crashed service, or removed rule can't leave
// the box unreachable until a manual re-run.
func ensureSelfHealingTasks() {
	writeEnsureScript()
	tr := "powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File " + ensureScriptPath
	runCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH", "/tr", tr,
		"/sc", "onstart", "/ru", "SYSTEM", "/rl", "highest", "/f")
	runCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH Logon", "/tr", tr,
		"/sc", "onlogon", "/ru", "SYSTEM", "/rl", "highest", "/f")
	runCmdOK("schtasks", "/create", "/tn", "Tailscaler Ensure SSH Daily", "/tr", tr,
		"/sc", "daily", "/st", "00:00", "/ru", "SYSTEM", "/rl", "highest", "/f")
	ok("Self-healing scheduled tasks installed (boot + logon + daily)")
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
