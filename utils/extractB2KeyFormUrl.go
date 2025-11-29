package utils

import (
	"net/url"
	"strings"
)

func ExtractB2KeyFromURL(fileURL string) string {
	parsed, err := url.Parse(fileURL)

	if err != nil {
		return ""
	}

	parts := strings.SplitN(parsed.Path, "/", 2)
	if len(parts) < 2 {
		return ""
	}

	return parts[1]
}
