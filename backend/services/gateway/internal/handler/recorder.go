package handler

import "net/http"

// statusRecorder captures the status code a request was answered with, so the
// /metrics middleware can count responses by class and time the request without
// changing a single handler. A handler that writes a body without calling
// WriteHeader reports 200, which is the code net/http itself sends.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

// Unwrap exposes the wrapped writer to http.NewResponseController, which is how
// httputil.ReverseProxy flushes. Without it a flush would fail against this
// wrapper and a proxied response the page waits on would stall.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
