package metadata

import (
	"fmt"
	"strings"
)

const MaxGroupDepth = 5

func NormalizeGroupPath(group string) (string, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return "", nil
	}
	parts := strings.Split(group, "/")
	if len(parts) > MaxGroupDepth {
		return "", fmt.Errorf("groups can be at most %d levels deep", MaxGroupDepth)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return "", fmt.Errorf("group levels cannot be empty")
		}
	}
	return strings.Join(parts, "/"), nil
}

func JoinLabelPath(group, label string) string {
	group = strings.TrimSpace(group)
	label = strings.TrimSpace(label)
	if group == "" {
		return label
	}
	if label == "" {
		return group + "/"
	}
	return group + "/" + label
}

func SplitLabelPath(raw string) (group, label string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	if strings.HasSuffix(raw, "/") {
		return "", "", fmt.Errorf("label required after /")
	}
	parts := strings.Split(raw, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return "", "", fmt.Errorf("group levels cannot be empty")
		}
	}
	if len(parts) == 1 {
		return "", parts[0], nil
	}
	label = parts[len(parts)-1]
	group, err = NormalizeGroupPath(strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return "", "", err
	}
	return group, label, nil
}

func LabelLeaf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		return strings.TrimSpace(raw[i+1:])
	}
	return raw
}

func LabelGroup(raw string) string {
	raw = strings.TrimSpace(raw)
	i := strings.LastIndex(raw, "/")
	if i < 0 {
		return ""
	}
	group, err := NormalizeGroupPath(raw[:i])
	if err != nil {
		return strings.TrimSpace(raw[:i])
	}
	return group
}
