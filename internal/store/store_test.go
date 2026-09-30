package store

import "testing"

func TestRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.LaunchDelayMs != 2000 || len(st.Servers) != 0 {
		t.Errorf("defaults = %+v", st)
	}

	st.BasePath = `C:\Games\Diablo II`
	st.Favourites = []string{"slashdiablo"}
	srv := st.Server("slashdiablo")
	srv.Components["maphack"] = "1.9.9"
	srv.ProfileVersion = 3

	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}

	again, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}

	got := again.Server("slashdiablo")
	if again.BasePath != st.BasePath || got.Components["maphack"] != "1.9.9" || got.ProfileVersion != 3 || got.Instances != 1 {
		t.Errorf("reloaded = %+v, server %+v", again, got)
	}
}

func TestCache(t *testing.T) {
	s, _ := Open(t.TempDir())

	if data, err := s.ReadCache("slashdiablo", "profile.json"); data != nil || err != nil {
		t.Errorf("missing cache = %q, %v", data, err)
	}

	if err := s.WriteCache("slashdiablo", "profile.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}

	if data, _ := s.ReadCache("slashdiablo", "profile.json"); string(data) != "{}" {
		t.Errorf("cache = %q", data)
	}

	if _, err := s.ServerDir("../escape"); err == nil {
		t.Error("server id escaped the data folder")
	}
}
