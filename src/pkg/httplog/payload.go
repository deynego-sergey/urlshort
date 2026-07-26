package httplog

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RequestPayload содержит сериализуемый дамп HTTP-запроса без Body.
type RequestPayload struct {
	Timestamp     time.Time           `gob:"timestamp"`
	ClientIP      string              `gob:"client_ip"`
	Method        string              `gob:"method"`
	Host          string              `gob:"host"`
	RequestURI    string              `gob:"request_uri"`
	Proto         string              `gob:"proto"`
	RemoteAddr    string              `gob:"remote_addr"`
	Header        map[string][]string `gob:"header"`
	URLScheme     string              `gob:"url_scheme"`
	URLHost       string              `gob:"url_host"`
	URLPath       string              `gob:"url_path"`
	URLRawQuery   string              `gob:"url_raw_query"`
	TargetURL     string              `gob:"target_url"`
	Form          map[string][]string `gob:"form"`
	PostForm      map[string][]string `gob:"post_form"`
	Cookies       map[string]string   `gob:"cookies"`
	ContentLength int64               `gob:"content_length"`
}

// Referer возвращает URL источника перехода из заголовка HTTP Referer.
func (p *RequestPayload) Referer() string {
	if p.Header == nil {
		return ""
	}
	if refs, ok := p.Header["Referer"]; ok && len(refs) > 0 {
		return refs[0]
	}
	if refs, ok := p.Header["referer"]; ok && len(refs) > 0 {
		return refs[0]
	}
	return ""
}

// NewRequestPayload создает сериализуемый экземпляр RequestPayload из *http.Request.
func NewRequestPayload(r *http.Request, targetURL string) *RequestPayload {
	cookiesMap := make(map[string]string, len(r.Cookies()))
	for _, c := range r.Cookies() {
		cookiesMap[c.Name] = c.Value
	}

	payload := &RequestPayload{
		Timestamp:     time.Now().UTC(),
		ClientIP:      ExtractRealIP(r),
		Method:        r.Method,
		Host:          r.Host,
		RequestURI:    r.RequestURI,
		Proto:         r.Proto,
		RemoteAddr:    r.RemoteAddr,
		Header:        r.Header,
		TargetURL:     targetURL,
		Form:          r.Form,
		PostForm:      r.PostForm,
		Cookies:       cookiesMap,
		ContentLength: r.ContentLength,
	}

	if r.URL != nil {
		payload.URLScheme = r.URL.Scheme
		payload.URLHost = r.URL.Host
		payload.URLPath = r.URL.Path
		payload.URLRawQuery = r.URL.RawQuery
	}

	return payload
}

func WritePayloadContext(ctx context.Context, w io.Writer, p *RequestPayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		enc := gob.NewEncoder(w)
		if err := enc.Encode(p); err != nil {
			return fmt.Errorf("encode payload failed: %w", err)
		}
		return nil
	}
}

func ReadPayloadContext(ctx context.Context, r io.Reader) (*RequestPayload, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		var p RequestPayload
		dec := gob.NewDecoder(r)
		if err := dec.Decode(&p); err != nil {
			return nil, err
		}
		return &p, nil
	}
}
