package winutil

import "testing"

func TestServiceState(t *testing.T) {
	running := `SERVICE_NAME: sshd
        TYPE               : 10  WIN32_OWN_PROCESS
        STATE              : 4  RUNNING
        WIN32_EXIT_CODE    : 0  (0x0)
        SERVICE_EXIT_CODE  : 0  (0x0)
        CHECKPOINT         : 0x0
        WAIT_HINT          : 0x0
`
	stopped := `SERVICE_NAME: sshd
        TYPE               : 10  WIN32_OWN_PROCESS
        STATE              : 1  STOPPED
        WIN32_EXIT_CODE    : 0  (0x0)
`
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"running", running, "RUNNING"},
		{"stopped", stopped, "STOPPED"},
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
