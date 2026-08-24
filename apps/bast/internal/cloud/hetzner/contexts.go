package hetzner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TokenContext struct {
	Name   string
	Token  string
	Source string // env, hcloud, file
}

func (c *Client) TokenContexts() ([]TokenContext, error) {
	seenToken := map[string]bool{}
	usedName := map[string]bool{}
	var out []TokenContext
	add := func(item TokenContext) {
		token := strings.TrimSpace(item.Token)
		name := strings.TrimSpace(item.Name)
		if token == "" || seenToken[token] {
			return
		}
		if name == "" {
			name = "default"
		}
		base := name
		for i := 2; usedName[strings.ToLower(name)]; i++ {
			name = fmt.Sprintf("%s_%d", base, i)
		}
		seenToken[token] = true
		usedName[strings.ToLower(name)] = true
		item.Token = token
		item.Name = name
		out = append(out, item)
	}

	if token := strings.TrimSpace(c.getenv(APIKeyEnv)); token != "" {
		name := strings.TrimSpace(c.getenv(ContextEnv))
		if name == "" {
			name = "default"
		}
		add(TokenContext{Name: name, Token: token, Source: "env"})
	}

	for _, path := range c.hcloudConfigPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, item := range parseHCloudTOML(data) {
			item.Source = "hcloud"
			add(item)
		}
	}

	if c.APIKey != "" {
		add(TokenContext{Name: "default", Token: c.APIKey, Source: "file"})
	} else {
		stored, err := c.ListStoredTokens()
		if err != nil {
			return nil, err
		}
		for _, item := range stored {
			add(item)
		}
	}

	return out, nil
}

func (c *Client) hcloudConfigPaths() []string {
	var paths []string
	seen := map[string]bool{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if c.ConfigPath != "" {
		add(c.ConfigPath)
		return paths
	}
	add(c.getenv(ConfigEnv))
	if dir, err := os.UserConfigDir(); err == nil {
		add(filepath.Join(dir, "hcloud", "cli.toml"))
	}
	if c.Home != "" {
		add(filepath.Join(c.Home, ".config", "hcloud", "cli.toml"))
	}
	return paths
}

func parseHCloudTOML(data []byte) []TokenContext {
	var out []TokenContext
	section := ""
	arrayName, arrayToken := "", ""
	flushArray := func() {
		if arrayToken != "" {
			name := arrayName
			if name == "" {
				name = "default"
			}
			out = append(out, TokenContext{Name: name, Token: arrayToken})
		}
		arrayName, arrayToken = "", ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			flushArray()
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			section = inner
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flushArray()
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = unquoteTOML(strings.TrimSpace(value))
		switch {
		case section == "contexts" && key == "name":
			arrayName = value
		case section == "contexts" && key == "token":
			arrayToken = value
		case strings.HasPrefix(section, "contexts.") && key == "token":
			name := strings.TrimPrefix(section, "contexts.")
			name = unquoteTOML(name)
			if name == "" {
				name = "default"
			}
			out = append(out, TokenContext{Name: name, Token: value})
		}
	}
	flushArray()
	return out
}

func unquoteTOML(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func filterContexts(contexts []TokenContext, names []string) []TokenContext {
	if len(names) == 0 {
		return contexts
	}
	var out []TokenContext
	for _, ctx := range contexts {
		if stringInFold(names, ctx.Name) {
			out = append(out, ctx)
		}
	}
	return out
}

func stringInFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
