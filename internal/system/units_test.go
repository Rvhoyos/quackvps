package system

import "testing"

// Real `systemctl show` output: one block per unit, properties in systemd's own
// order, ExecStart in its verbose form.
const showOutput = `ExecStart={ path=/usr/bin/screen ; argv[]=/usr/bin/screen -S survival -dm bash -lc ./run.sh ; ignore_errors=no }
WorkingDirectory=/home/ubuntu/mcserver/survival
User=ubuntu
Id=mc-survival.service

ExecStart={ path=/usr/sbin/sshd ; argv[]=/usr/sbin/sshd -D $SSHD_OPTS ; ignore_errors=no }
WorkingDirectory=
User=
Id=ssh.service

ExecStart={ path=/usr/bin/java ; argv[]=/usr/bin/java -Xmx4G -jar /srv/minecraft/server.jar nogui ; ignore_errors=no }
WorkingDirectory=
User=minecraft
Id=minecraft.service
`

func TestParseShow(t *testing.T) {
	units := parseShow(showOutput)
	if len(units) != 3 {
		t.Fatalf("got %d units, want 3", len(units))
	}
	first := units[0]
	if first.Name != "mc-survival.service" {
		t.Errorf("name = %q, want mc-survival.service", first.Name)
	}
	if first.WorkingDir != "/home/ubuntu/mcserver/survival" {
		t.Errorf("working dir = %q", first.WorkingDir)
	}
	if first.User != "ubuntu" {
		t.Errorf("user = %q, want ubuntu", first.User)
	}
	if units[1].User != "" {
		t.Errorf("ssh.service user = %q, want empty (runs as root)", units[1].User)
	}
}

func TestServiceNames(t *testing.T) {
	out := "mc-survival.service                    enabled enabled\n" +
		"getty@.service                         enabled enabled\n" +
		"ssh.service                            enabled enabled\n"
	got := serviceNames(out)
	want := []string{"mc-survival.service", "ssh.service"} // templates are not startable
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUnitsForInstance(t *testing.T) {
	units := parseShow(showOutput)
	tests := []struct {
		dir  string
		want []string
	}{
		{"/home/ubuntu/mcserver/survival", []string{"mc-survival.service"}}, // by working directory
		{"/home/ubuntu/mcserver/survival/", []string{"mc-survival.service"}},
		{"/srv/minecraft", []string{"minecraft.service"}}, // by command line
		{"/home/ubuntu/mcserver/creative", nil},
	}
	for _, tt := range tests {
		got := UnitsForInstance(units, tt.dir)
		if len(got) != len(tt.want) {
			t.Fatalf("UnitsForInstance(%q) = %v, want %v", tt.dir, got, tt.want)
		}
		for i := range tt.want {
			if got[i].Name != tt.want[i] {
				t.Errorf("UnitsForInstance(%q)[%d] = %q, want %q", tt.dir, i, got[i].Name, tt.want[i])
			}
		}
	}
}

func TestScreenSession(t *testing.T) {
	tests := []struct {
		execStart string
		want      string
	}{
		{"{ path=/usr/bin/screen ; argv[]=/usr/bin/screen -S survival -dm bash -lc ./run.sh }", "survival"},
		{"{ path=/usr/bin/java ; argv[]=/usr/bin/java -jar server.jar nogui }", ""},
	}
	for _, tt := range tests {
		if got := ScreenSession(tt.execStart); got != tt.want {
			t.Errorf("ScreenSession(%q) = %q, want %q", tt.execStart, got, tt.want)
		}
	}
}
