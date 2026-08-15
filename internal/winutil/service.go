package winutil

import (
	"strings"
	"time"
)

// ServiceExists reports whether a Windows service with the given name is present.
func ServiceExists(name string) bool {
	return RunCmdOK("sc.exe", "query", name) == nil
}

// ServiceRunning reports whether the service currently exists and is RUNNING.
func ServiceRunning(name string) bool {
	out, err := RunCmd("sc.exe", "query", name)
	if err != nil {
		return false
	}
	return ServiceState(out) == "RUNNING"
}

// WaitExists polls until the service is present or the timeout elapses.
func WaitExists(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if ServiceExists(name) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// WaitRunning polls until the service is RUNNING or the timeout elapses.
// A freshly started service transitions through START_PENDING, so a single
// snapshot taken right after `sc start` is not a reliable readiness check.
func WaitRunning(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if ServiceRunning(name) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// scCode scans `sc` output for the first field whose numeric code is in want.
// Labels are localized (STATE / ESTADO, START_TYPE / TIPO_INICIO, ...), so the
// numeric code that uniquely identifies each field is matched instead.
func scCode(output string, want map[string]bool) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[1] == ":" && want[fields[2]] {
			return fields[2]
		}
	}
	return ""
}

// ServiceState extracts the service state from `sc query` output. Every SCM
// state code is mapped so reports reflect reality (e.g. START_PENDING while a
// service is coming up) instead of collapsing everything to UNKNOWN.
func ServiceState(output string) string {
	switch scCode(output, map[string]bool{
		"1": true, "2": true, "3": true, "4": true,
		"5": true, "6": true, "7": true,
	}) {
	case "1":
		return "STOPPED"
	case "2":
		return "START_PENDING"
	case "3":
		return "STOP_PENDING"
	case "4":
		return "RUNNING"
	case "5":
		return "CONTINUE_PENDING"
	case "6":
		return "PAUSE_PENDING"
	case "7":
		return "PAUSED"
	}
	return "UNKNOWN"
}

// StartType extracts the start type from `sc qc` output:
// 2=auto (incl. delayed), 3=demand, 4=disabled.
func StartType(output string) string {
	switch scCode(output, map[string]bool{"2": true, "3": true, "4": true}) {
	case "2":
		return "AUTO_START"
	case "3":
		return "DEMAND_START"
	case "4":
		return "DISABLED"
	}
	return "?"
}
