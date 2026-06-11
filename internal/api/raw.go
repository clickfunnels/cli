package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// RawResponse is the undecoded result of a raw request.
type RawResponse struct {
	StatusCode int
	Body       []byte
}

// RawRequest issues an authenticated request to an arbitrary path (relative to
// baseURL) and returns the raw response, body intact even on non-2xx. It's the
// escape hatch behind `cf api` for endpoints the typed client doesn't cover, so
// unlike the generated client it does not normalize errors — the caller sees
// whatever came back.
func RawRequest(ctx context.Context, baseURL, token, method, path string, body []byte) (*RawResponse, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	url := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &RawResponse{StatusCode: resp.StatusCode, Body: b}, nil
}

// ToContactUpdate converts create params into the update body type. The two are
// structurally identical in the spec; rather than copy fields by hand (which
// would be exactly the duplication we're avoiding), we round-trip through JSON,
// so it tracks any future divergence automatically.
func ToContactUpdate(p ContactParameters) ContactParametersUpdate {
	var u ContactParametersUpdate
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &u)
	return u
}
