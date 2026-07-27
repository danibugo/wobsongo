package data

import "net/http"

// HTTPClient defines behavior for sending HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
