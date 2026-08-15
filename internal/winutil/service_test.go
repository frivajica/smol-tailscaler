package winutil

import "testing"

func TestServiceState(t *testing.T) {
	state := func(code, label string) string {
		return "SERVICE_NAME: sshd\n" +
			"        TYPE               : 10  WIN32_OWN_PROCESS\n" +
			"        STATE              : " + code + "  " + label + "\n" +
			"        WIN32_EXIT_CODE    : 0  (0x0)\n"
	}
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"running", state("4", "RUNNING"), "RUNNING"},
		{"stopped", state("1", "STOPPED"), "STOPPED"},
		{"start pending", state("2", "START_PENDING"), "START_PENDING"},
		{"stop pending", state("3", "STOP_PENDING"), "STOP_PENDING"},
		{"continue pending", state("5", "CONTINUE_PENDING"), "CONTINUE_PENDING"},
		{"pause pending", state("6", "PAUSE_PENDING"), "PAUSE_PENDING"},
		{"paused", state("7", "PAUSED"), "PAUSED"},
		{"no match", "SERVICE_NAME: sshd\n        TYPE : 10\n", "UNKNOWN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ServiceState(tc.output); got != tc.want {
				t.Errorf("ServiceState() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStartType(t *testing.T) {
	build := func(code string) string {
		return "SERVICE_NAME: sshd\n" +
			"        TYPE               : 10  WIN32_OWN_PROCESS\n" +
			"        START_TYPE         : " + code + "   AUTO_START\n" +
			"        ERROR_CONTROL      : 1   NORMAL\n"
	}
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"auto", build("2"), "AUTO_START"},
		{"demand", build("3"), "DEMAND_START"},
		{"disabled", build("4"), "DISABLED"},
		{"no match", "SERVICE_NAME: sshd\n        START_TYPE : 5\n", "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StartType(tc.output); got != tc.want {
				t.Errorf("StartType() = %q, want %q", got, tc.want)
			}
		})
	}
}
