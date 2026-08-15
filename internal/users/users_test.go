package users

import "testing"

func TestValidName(t *testing.T) {
	valid := []string{"admin", "frivajica", "web.server.01", "super-admin", "A.B_C-1"}
	invalid := []string{"", "ad min", "user'name", `user"name`, "user;name", "áli", "user\\name", "us,er"}
	for _, name := range valid {
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false, want true", name)
		}
	}
	for _, name := range invalid {
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true, want false", name)
		}
	}
}
