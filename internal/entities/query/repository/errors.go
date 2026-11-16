// Package repository contains shared query-provider request and response helpers.
package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"syscall"

	anthropicsdk "github.com/liushuangls/go-anthropic/v2"
	openaisdk "github.com/sashabaranov/go-openai"
)

// providerError keeps the cause available without exposing its potentially sensitive text.
type providerError struct {
	message string
	cause   error
}

func (e *providerError) Error() string { return e.message }

func (e *providerError) Unwrap() error { return e.cause }

// WrapNetworkError returns a safe provider-facing error, or nil for a nil cause.
// The original error remains available through errors.Is and errors.As.
// providerName must be a trusted display name, not a URL or server-supplied value.
func WrapNetworkError(providerName string, originalErr error) error {
	if originalErr == nil {
		return nil
	}

	return &providerError{
		message: networkErrorMessage(providerName, originalErr),
		cause:   originalErr,
	}
}

// WrapResponseError describes an unreadable or invalid provider response without its contents.
// A nil cause returns nil; non-nil causes remain available through errors.Is and errors.As.
// providerName must be a trusted display name.
func WrapResponseError(providerName string, cause error) error {
	if cause == nil {
		return nil
	}
	return &providerError{
		message: providerName + " returned an invalid response. Try again or select another model",
		cause:   cause,
	}
}

// HTTPStatusError describes a failed provider response without including its body or URL.
// providerName must be a trusted display name; status is the HTTP response status code.
func HTTPStatusError(providerName string, status int) error {
	return fmt.Errorf("%s HTTP %d: %s", providerName, status, httpStatusAdvice(status))
}

func httpStatusAdvice(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid request. Check the model, request size and provider settings with `zev setup`"
	case http.StatusUnauthorized:
		return "authentication failed. Check your API key with `zev setup`"
	case http.StatusPaymentRequired:
		return "payment required. Check billing and available credits with your provider"
	case http.StatusForbidden:
		return "permission denied. Check API key permissions and access to the model"
	case http.StatusNotFound:
		return "resource not found. Check the model and base URL with `zev setup`"
	case http.StatusRequestTimeout:
		return "request timed out. Check your network, then try again"
	case http.StatusConflict:
		return "request conflict. Wait briefly and try again"
	case http.StatusRequestEntityTooLarge:
		return "request too large. Shorten the prompt or conversation history"
	case http.StatusTooManyRequests:
		return "rate limit or quota reached. Wait and retry; check provider usage limits and billing"
	default:
		if status >= http.StatusInternalServerError && status < 600 {
			return "provider temporarily unavailable. Try again later or select another provider"
		}
		return "request failed. Check provider settings with `zev setup`"
	}
}

func sdkErrorMessage(providerName string, err error) string {
	if apiErr, ok := errors.AsType[*openaisdk.APIError](err); ok {
		if apiErr.HTTPStatusCode > 0 {
			if code, ok := apiErr.Code.(string); ok && code == "insufficient_quota" {
				return fmt.Sprintf("%s HTTP %d: %s", providerName, apiErr.HTTPStatusCode, httpStatusAdvice(http.StatusPaymentRequired))
			}
			return HTTPStatusError(providerName, apiErr.HTTPStatusCode).Error()
		}
	}
	if reqErr, ok := errors.AsType[*openaisdk.RequestError](err); ok && reqErr.HTTPStatusCode > 0 {
		return HTTPStatusError(providerName, reqErr.HTTPStatusCode).Error()
	}
	if reqErr, ok := errors.AsType[*anthropicsdk.RequestError](err); ok && reqErr.StatusCode > 0 {
		return HTTPStatusError(providerName, reqErr.StatusCode).Error()
	}
	if apiErr, ok := errors.AsType[*anthropicsdk.APIError](err); ok {
		// This SDK exposes the API error type but not the original HTTP status.
		status := 0
		switch apiErr.Type {
		case anthropicsdk.ErrTypeInvalidRequest:
			status = http.StatusBadRequest
		case anthropicsdk.ErrTypeAuthentication:
			status = http.StatusUnauthorized
		case anthropicsdk.ErrTypePermission:
			status = http.StatusForbidden
		case anthropicsdk.ErrTypeNotFound:
			status = http.StatusNotFound
		case anthropicsdk.ErrTypeTooLarge:
			status = http.StatusRequestEntityTooLarge
		case anthropicsdk.ErrTypeRateLimit:
			status = http.StatusTooManyRequests
		case anthropicsdk.ErrTypeApi, anthropicsdk.ErrTypeOverloaded:
			status = http.StatusServiceUnavailable
		}
		return providerName + " API error: " + httpStatusAdvice(status)
	}
	return ""
}

func networkErrorMessage(providerName string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return providerName + " did not respond within timeout. Check your network or try again"
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Sprintf("request to %s was cancelled", providerName)
	}
	if message := sdkErrorMessage(providerName, err); message != "" {
		return message
	}
	dnsErr, dnsOK := errors.AsType[*net.DNSError](err)
	if dnsOK && dnsErr != nil {
		return fmt.Sprintf("cannot resolve %s hostname. Check the base URL, DNS or internet connection", providerName)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Sprintf("connection to %s refused. Check the base URL, local server and proxy/VPN", providerName)
	}
	if errors.Is(err, syscall.ETIMEDOUT) {
		return providerName + " connection timed out. Check firewall or network"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return providerName + " request timed out. Check your network or increase timeout"
	}

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return fmt.Sprintf("connection to %s was reset. Check your network and proxy/VPN, then try again", providerName)
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameMismatch x509.HostnameError
	var invalidCertificate x509.CertificateInvalidError
	var verificationFailed *tls.CertificateVerificationError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameMismatch) || errors.As(err, &invalidCertificate) || errors.As(err, &verificationFailed) {
		return providerName + " TLS certificate verification failed. Check the base URL, system clock and trusted certificates"
	}
	recordHeader, recordHeaderOK := errors.AsType[tls.RecordHeaderError](err)
	if recordHeaderOK {
		_ = recordHeader
		return providerName + " TLS connection failed. Check HTTPS settings and proxy/VPN"
	}

	return providerName + " API error: request failed. Check provider settings with `zev setup` and your network"
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
