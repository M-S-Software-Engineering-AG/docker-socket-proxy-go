package proxy

import "testing"

func TestStripVersion(t *testing.T) {
	cases := map[string]string{
		"/v1.54/containers/json": "/containers/json",
		"/v1/containers/json":    "/containers/json",
		"/containers/json":       "/containers/json",
		"/v1.54":                 "/",
		"/v1.54/":                "/",
		"/_ping":                 "/_ping",
		"/version":               "/version", // not a version prefix (no digits after v with /)
	}
	for in, want := range cases {
		if got := StripVersion(in); got != want {
			t.Errorf("StripVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSection(t *testing.T) {
	cases := map[string]string{
		"/containers/json":            "CONTAINERS",
		"/v1.54/containers/abc/start": "CONTAINERS",
		"/_ping":                      "PING",
		"/v1.54/_ping":                "PING",
		"/info":                       "INFO",
		"/version":                    "VERSION",
		"/system/df":                  "SYSTEM",
		"/build":                      "BUILD",
		"/build/prune":                "BUILD",
		"/exec/abc/start":             "EXEC",
		"/containers/abc/exec":        "CONTAINERS", // exec creation lives under containers
		"/distribution/abc/json":      "DISTRIBUTION",
		"/session":                    "SESSION",
		"/grpc":                       "GRPC",
		"/unknown":                    "",
		"/":                           "",
	}
	for path, want := range cases {
		if got := Section(path); got != want {
			t.Errorf("Section(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestDecide(t *testing.T) {
	cfg := &Config{
		Sections: map[string]bool{"CONTAINERS": true, "PING": true},
		Readonly: true,
	}

	tests := []struct {
		method, path string
		want         Decision
		status       int
		section      string
	}{
		{"GET", "/containers/json", Allow, 0, "CONTAINERS"},
		{"HEAD", "/containers/json", Allow, 0, "CONTAINERS"},
		{"POST", "/containers/create", DenyReadonly, 405, "CONTAINERS"},
		{"DELETE", "/v1.54/containers/abc", DenyReadonly, 405, "CONTAINERS"},
		{"GET", "/images/json", DenyDisabled, 403, "IMAGES"},
		{"GET", "/_ping", Allow, 0, "PING"},
		{"GET", "/whatever", DenyUnknown, 404, ""},
	}
	for _, tc := range tests {
		r := cfg.Decide(tc.method, tc.path)
		if r.Decision != tc.want || r.Status != tc.status || r.Section != tc.section {
			t.Errorf("Decide(%s %s) = %+v, want decision=%s status=%d section=%s",
				tc.method, tc.path, r, tc.want, tc.status, tc.section)
		}
	}
}

func TestDecideReadonlyOff(t *testing.T) {
	cfg := &Config{Sections: map[string]bool{"CONTAINERS": true}, Readonly: false}
	r := cfg.Decide("POST", "/containers/create")
	if r.Decision != Allow {
		t.Errorf("expected Allow when readonly off, got %+v", r)
	}
}

func TestAllSectionsCoverage(t *testing.T) {
	want := []string{
		"AUTH", "BUILD", "COMMIT", "CONFIGS", "CONTAINERS", "DISTRIBUTION",
		"EVENTS", "EXEC", "GRPC", "IMAGES", "INFO", "NETWORKS", "NODES",
		"PING", "PLUGINS", "SECRETS", "SERVICES", "SESSION", "SWARM",
		"SYSTEM", "TASKS", "VERSION", "VOLUMES",
	}
	got := AllSections()
	if len(got) != len(want) {
		t.Fatalf("AllSections len = %d, want %d (%v)", len(got), len(want), got)
	}
	have := map[string]bool{}
	for _, s := range got {
		have[s] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("AllSections missing %s", w)
		}
	}
}
