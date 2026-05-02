package server

import (
	"fmt"
	"strings"
)

var hardcodedControlBuildChannel = "stable"

func normalizeUpgradeChannel(raw string) (string, error) {
	channel := strings.ToLower(strings.TrimSpace(raw))
	if channel == "" {
		return "stable", nil
	}
	if channel != "stable" && channel != "beta" {
		return "", fmt.Errorf("invalid channel")
	}
	return channel, nil
}

func controlBuildChannel() string {
	ch, err := normalizeUpgradeChannel(hardcodedControlBuildChannel)
	if err != nil {
		return "stable"
	}
	return ch
}
