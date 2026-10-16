package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"syscall"
)

// WrapNetworkError оборачивает сетевую ошибку в человекочитаемое сообщение.
func WrapNetworkError(providerName string, originalErr error) error {
	if originalErr == nil {
		return nil
	}

	// Context timeout/cancelled
	if errors.Is(originalErr, context.DeadlineExceeded) {
		return fmt.Errorf("%s did not respond within timeout. Check your network or try again", providerName)
	}
	if errors.Is(originalErr, context.Canceled) {
		return fmt.Errorf("request to %s was cancelled", providerName)
	}

	// DNS errors
	if _, ok := errors.AsType[*net.DNSError](originalErr); ok {
		return fmt.Errorf("cannot resolve %s hostname. Check your DNS or internet connection", providerName)
	}

	// Connection refused
	if opErr, ok := errors.AsType[*net.OpError](originalErr); ok {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return fmt.Errorf("connection to %s refused. Check if proxy/VPN is required", providerName)
		}
		if errors.Is(opErr.Err, syscall.ETIMEDOUT) {
			return fmt.Errorf("%s connection timed out. Check firewall or network", providerName)
		}
	}

	// URL parse errors
	if urlErr, ok := errors.AsType[*url.Error](originalErr); ok {
		if urlErr.Timeout() {
			return fmt.Errorf("%s request timed out. Check your network or increase timeout", providerName)
		}
		if strings.Contains(urlErr.Error(), "no such host") {
			return fmt.Errorf("cannot reach %s. Check internet connection", providerName)
		}
	}

	// Fallback: wrap original
	return fmt.Errorf("%s API error: %w", providerName, originalErr)
}

// ExtractJSON извлекает чистый JSON из markdown-блоков (```json...```) или escape-тегов (<escape>...</escape>).
// Если ни того, ни другого нет — возвращает текст как есть.
func ExtractJSON(text string) string {
	text = strings.TrimSpace(text)

	// 1. Убираем <escape>...</escape> (dotall mode через (?s))
	escapePattern := regexp.MustCompile(`(?s)<escape>(.*?)</escape>`)
	if match := escapePattern.FindStringSubmatch(text); len(match) > 1 {
		text = strings.TrimSpace(match[1])
	}

	// 2. Убираем markdown ```json...```
	const jsonPrefix = "```json"
	const jsonSuffix = "```"

	startIdx := strings.Index(text, jsonPrefix)
	if startIdx != -1 {
		startIdx += len(jsonPrefix)
		endIdx := strings.Index(text[startIdx:], jsonSuffix)
		if endIdx != -1 {
			text = strings.TrimSpace(text[startIdx : startIdx+endIdx])
		}
	}

	return text
}
