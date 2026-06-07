package auth

import "net/http"

// clients.go exists to mock upstream interfaces our code depends on.

//go:generate mockgen -source=clients.go -destination=clients.mock.gen.go -package=auth

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}
