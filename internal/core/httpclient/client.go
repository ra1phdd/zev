package httpclient

import (
	"net/http"
	"time"
)

// New создаёт HTTP клиент с настроенными заголовками.
func New(headers map[string]string) *http.Client {
	if headers == nil {
		headers = make(map[string]string)
	}

	// Устанавливаем дефолтный User-Agent, если он не задан
	if _, ok := headers["User-Agent"]; !ok {
		headers["User-Agent"] = "zev/1.0"
	}

	transport := &headersTransport{
		headers: headers,
		base:    http.DefaultTransport,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// headersTransport добавляет кастомные заголовки к каждому запросу.
type headersTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *headersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())

	for key, value := range t.headers {
		reqCopy.Header[key] = []string{value}
	}

	return t.base.RoundTrip(reqCopy)
}
