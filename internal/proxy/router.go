package proxy

import (
	"regexp"
	"strings"
)

type Decision string

const (
	Allow         Decision = "allow"
	DenyDisabled  Decision = "deny:section_disabled"
	DenyReadonly  Decision = "deny:readonly"
	DenyUnknown   Decision = "deny:unknown_path"
)

type Result struct {
	Decision Decision
	Section  string
	Status   int
}

type Config struct {
	Sections map[string]bool
	Readonly bool
}

var sectionPrefixes = []struct {
	prefix string
	env    string
}{
	{"/_ping", "PING"},
	{"/auth", "AUTH"},
	{"/build", "BUILD"},
	{"/commit", "COMMIT"},
	{"/configs", "CONFIGS"},
	{"/containers", "CONTAINERS"},
	{"/distribution", "DISTRIBUTION"},
	{"/events", "EVENTS"},
	{"/exec", "EXEC"},
	{"/grpc", "GRPC"},
	{"/images", "IMAGES"},
	{"/info", "INFO"},
	{"/networks", "NETWORKS"},
	{"/nodes", "NODES"},
	{"/plugins", "PLUGINS"},
	{"/secrets", "SECRETS"},
	{"/services", "SERVICES"},
	{"/session", "SESSION"},
	{"/swarm", "SWARM"},
	{"/system", "SYSTEM"},
	{"/tasks", "TASKS"},
	{"/version", "VERSION"},
	{"/volumes", "VOLUMES"},
}

// AllSections returns the canonical env-var names of every gated section.
func AllSections() []string {
	out := make([]string, 0, len(sectionPrefixes))
	seen := map[string]bool{}
	for _, s := range sectionPrefixes {
		if !seen[s.env] {
			seen[s.env] = true
			out = append(out, s.env)
		}
	}
	return out
}

var versionRe = regexp.MustCompile(`^/v[0-9]+(\.[0-9]+)?(/|$)`)

// StripVersion removes a leading "/vX" or "/vX.Y" Docker API version segment.
func StripVersion(p string) string {
	loc := versionRe.FindStringIndex(p)
	if loc == nil {
		return p
	}
	end := loc[1]
	if end >= len(p) {
		return "/"
	}
	return p[end-1:] // include the trailing "/" that the regex consumed
}

// Section returns the env-var name for the section that owns this path,
// or "" if no section claims it.
func Section(path string) string {
	p := StripVersion(path)
	for _, s := range sectionPrefixes {
		if p == s.prefix || strings.HasPrefix(p, s.prefix+"/") {
			return s.env
		}
	}
	return ""
}

// Decide applies the gate + readonly logic and returns the routing result.
func (c *Config) Decide(method, path string) Result {
	sec := Section(path)
	if sec == "" {
		return Result{Decision: DenyUnknown, Status: 404}
	}
	if !c.Sections[sec] {
		return Result{Decision: DenyDisabled, Section: sec, Status: 403}
	}
	if c.Readonly && method != "GET" && method != "HEAD" {
		return Result{Decision: DenyReadonly, Section: sec, Status: 405}
	}
	return Result{Decision: Allow, Section: sec}
}
