package paths

import "testing"

func TestCheck(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{"Game.exe", true},
		{"data/global/excel/armor.txt", true},
		{"a..b.dll", true},
		{"ProjectD2/BH.cfg", true},

		{"", false},
		{"/Game.exe", false},
		{"../Game.exe", false},
		{"data/../../Game.exe", false},
		{"./Game.exe", false},
		{`data\Game.exe`, false},
		{"C:/Windows/evil.dll", false},
		{"Game.exe:stream", false},
		{"data//x.txt", false},
		{"data/", false},
		{"Game.exe.", false},
		{"Game.exe ", false},
		{"CON", false},
		{"nul.txt", false},
		{"data/com1.dll", false},
		{"a\x01b", false},
	}

	for _, tt := range tests {
		err := Check(tt.path)
		if (err == nil) != tt.ok {
			t.Errorf("Check(%q) = %v, want ok=%v", tt.path, err, tt.ok)
		}
	}
}

func TestCheckPattern(t *testing.T) {
	tests := []struct {
		pattern string
		ok      bool
	}{
		{"version.txt", true},
		{"Debug*.log", true},
		{"logs/client-??.log", true},
		{"*.log", true},

		{"*", false},
		{"*.*", false},
		{"logs/*", false},
		{"*/client.log", false},
		{"../*.log", false},
		{"/client.log", false},
		{"logs/", false},
		{"client*.log.", false},
		{"nul*.txt", true}, // NULx.txt is a plain name; NUL.txt itself can't exist
		{"nul.txt", false},
		{"[ab].log", false},
	}

	for _, tt := range tests {
		err := CheckPattern(tt.pattern)
		if (err == nil) != tt.ok {
			t.Errorf("CheckPattern(%q) = %v, want ok=%v", tt.pattern, err, tt.ok)
		}
	}
}

func TestKey(t *testing.T) {
	if Key("Game.EXE") != Key("game.exe") {
		t.Error("keys differ only by case")
	}
}
