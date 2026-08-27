package vercel

import (
	"os"
	"strings"
	"unicode"
)

// ParseProjectList splits project IDs or names from one or more user values.
// Commas, semicolons, and whitespace separate entries.
func ParseProjectList(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range values {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || unicode.IsSpace(r)
		}) {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

func (c *Client) ResolveProjects() []string {
	env := ""
	if c != nil {
		env = os.Getenv(ProjectEnv)
		return ParseProjectList(strings.Join(c.ProjectIDs, ","), c.ProjectID, env)
	}
	return ParseProjectList(os.Getenv(ProjectEnv))
}

func (c *Client) ResolveProject() string {
	projects := c.ResolveProjects()
	if len(projects) == 0 {
		return ""
	}
	return projects[0]
}
