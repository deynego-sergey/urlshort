package httplog

import (
	"net/http"
	"time"
)

// RequestPayload содержит сериализуемый дамп HTTP-запроса без Body
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
	Form          map[string][]string `gob:"form"`
	PostForm      map[string][]string `gob:"post_form"`
	Cookies       map[string]string   `gob:"cookies"`
	ContentLength int64               `gob:"content_length"`
}

// NewRequestPayload создает сериализуемый экзепляр RequestPayload из *http.Request
func NewRequestPayload(r *http.Request) *RequestPayload {
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
