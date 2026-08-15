package winutil

import "strings"

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

// ServiceState extracts RUNNING / STOPPED from `sc query` output.
func ServiceState(output string) string {
	switch scCode(output, map[string]bool{"1": true, "4": true}) {
	case "1":
		return "STOPPED"
	case "4":
		return "RUNNING"
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
