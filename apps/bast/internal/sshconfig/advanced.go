package sshconfig

import (
	"fmt"
	"strconv"
	"strings"
)

type AdvancedSettings struct {
	ProxyJump           string   `json:"proxyJump,omitempty"`
	ForwardAgent        string   `json:"forwardAgent,omitempty"`
	RemoteCommand       string   `json:"remoteCommand,omitempty"`
	RequestTTY          string   `json:"requestTTY,omitempty"`
	SetEnv              []string `json:"setEnv,omitempty"`
	LocalForwards       []string `json:"localForwards,omitempty"`
	RemoteForwards      []string `json:"remoteForwards,omitempty"`
	DynamicForward      string   `json:"dynamicForward,omitempty"`
	ServerAliveInterval string   `json:"serverAliveInterval,omitempty"`
	Compression         string   `json:"compression,omitempty"`
	Custom              []string `json:"custom,omitempty"`
}

func ParseAdvanced(extras []string, proxyJump string) AdvancedSettings {
	settings := AdvancedSettings{ProxyJump: strings.TrimSpace(proxyJump)}
	for _, option := range extras {
		line := strings.TrimSpace(option)
		if line == "" {
			continue
		}
		parts, err := fields(line)
		if err != nil || len(parts) == 0 {
			settings.Custom = append(settings.Custom, line)
			continue
		}
		name := strings.ToLower(parts[0])
		value := strings.TrimSpace(strings.Join(parts[1:], " "))
		switch name {
		case "forwardagent":
			settings.ForwardAgent = strings.ToLower(value)
		case "remotecommand":
			settings.RemoteCommand = value
		case "requesttty":
			settings.RequestTTY = strings.ToLower(value)
		case "setenv":
			if value != "" {
				settings.SetEnv = append(settings.SetEnv, value)
			}
		case "localforward":
			if value != "" {
				settings.LocalForwards = append(settings.LocalForwards, value)
			}
		case "remoteforward":
			if value != "" {
				settings.RemoteForwards = append(settings.RemoteForwards, value)
			}
		case "dynamicforward":
			settings.DynamicForward = value
		case "serveraliveinterval":
			settings.ServerAliveInterval = value
		case "compression":
			settings.Compression = strings.ToLower(value)
		default:
			settings.Custom = append(settings.Custom, line)
		}
	}
	return settings
}

func (s AdvancedSettings) ExtraOptions() []string {
	var out []string
	if s.ForwardAgent != "" {
		out = append(out, "ForwardAgent "+s.ForwardAgent)
	}
	if s.RemoteCommand != "" {
		out = append(out, "RemoteCommand "+quoteSSHValue(s.RemoteCommand))
	}
	if s.RequestTTY != "" {
		out = append(out, "RequestTTY "+s.RequestTTY)
	}
	for _, env := range s.SetEnv {
		if env = strings.TrimSpace(env); env != "" {
			out = append(out, "SetEnv "+quoteSSHValue(env))
		}
	}
	for _, forward := range s.LocalForwards {
		if forward = strings.TrimSpace(forward); forward != "" {
			out = append(out, "LocalForward "+forward)
		}
	}
	for _, forward := range s.RemoteForwards {
		if forward = strings.TrimSpace(forward); forward != "" {
			out = append(out, "RemoteForward "+forward)
		}
	}
	if s.DynamicForward != "" {
		out = append(out, "DynamicForward "+strings.TrimSpace(s.DynamicForward))
	}
	if s.ServerAliveInterval != "" {
		out = append(out, "ServerAliveInterval "+strings.TrimSpace(s.ServerAliveInterval))
	}
	if s.Compression != "" {
		out = append(out, "Compression "+s.Compression)
	}
	out = append(out, s.Custom...)
	return out
}

func quoteSSHValue(value string) string {
	if !strings.ContainsAny(value, " \t#\\\"") {
		return value
	}
	return configValue(value)
}

func ParseSetEnvList(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var out []string
	for _, chunk := range strings.FieldsFunc(input, func(r rune) bool { return r == '\n' || r == ';' }) {
		if line := strings.TrimSpace(chunk); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func FormatSetEnvList(values []string) string {
	return strings.Join(values, "; ")
}

func ParseForwardList(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var out []string
	for _, chunk := range strings.FieldsFunc(input, func(r rune) bool { return r == '\n' || r == ';' }) {
		if line := strings.TrimSpace(chunk); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func FormatForwardList(values []string) string {
	return strings.Join(values, "; ")
}

func ValidateAdvanced(settings AdvancedSettings) error {
	for name, value := range map[string]string{
		"proxy jump": settings.ProxyJump, "remote command": settings.RemoteCommand,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s cannot contain a newline or null byte", name)
		}
	}
	if settings.ProxyJump != "" && strings.ContainsAny(settings.ProxyJump, " \t") {
		return fmt.Errorf("proxy jump cannot contain whitespace")
	}
	for name, value := range map[string]struct {
		value   string
		allowed map[string]bool
	}{
		"forward agent": {settings.ForwardAgent, map[string]bool{"": true, "yes": true, "no": true}},
		"compression":   {settings.Compression, map[string]bool{"": true, "yes": true, "no": true}},
		"request TTY":   {settings.RequestTTY, map[string]bool{"": true, "auto": true, "yes": true, "no": true, "force": true}},
	} {
		if !value.allowed[strings.ToLower(strings.TrimSpace(value.value))] {
			return fmt.Errorf("%s has an unsupported value", name)
		}
	}
	if value := strings.TrimSpace(settings.DynamicForward); value != "" {
		if strings.ContainsAny(value, " \t\r\n\x00") {
			return fmt.Errorf("dynamic forward must be a port or bind address and port")
		}
		port := value
		if colon := strings.LastIndex(value, ":"); colon >= 0 {
			port = value[colon+1:]
		}
		parsed, err := strconv.Atoi(port)
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("dynamic forward port must be between 1 and 65535")
		}
	}
	if value := strings.TrimSpace(settings.ServerAliveInterval); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return fmt.Errorf("server alive interval must be a non-negative integer")
		}
	}
	for _, env := range settings.SetEnv {
		if strings.ContainsAny(env, "\r\n\x00") {
			return fmt.Errorf("environment variables cannot contain a newline or null byte")
		}
	}
	return validateExtraOptions(settings.Custom)
}
