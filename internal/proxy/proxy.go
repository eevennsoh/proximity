package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"

	"github.com/go-chi/chi"
)

type server struct {
	Options
	*config.Config

	router     *chi.Mux
	httpServer *http.Server

	template *Template
}

func New(cfg *config.Config, options Options) Interface {
	router := chi.NewRouter()

	httpServer := &http.Server{
		Addr:    fmt.Sprint(":", options.Port),
		Handler: router,
	}

	return &server{
		Options:    options,
		Config:     cfg,
		router:     router,
		httpServer: httpServer,
		template:   newTemplate(options.Logger),
	}
}

func (s *server) RunServer(ctx context.Context) {
	s.Logger.Printf("starting http server on port %d", s.Options.Port)

	s.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// TODO: REPLACE THIS, THIS IS TEMPORARY
	s.router.Get("/openai/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{\"object\": \"list\",\"data\": [{\"id\": \"gpt-4.1-2025-04-14\",\"object\": \"model\",\"created\": 1686935002,\"owned_by\": \"atlassian\"}]}"))
	})

	s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Not found:", r.URL)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	})

	for _, supportedUri := range s.SupportedUris {
		endpointProxyCfg, err := s.buildEndpointProxyConfig(supportedUri)
		if err != nil {
			s.Logger.Fatal(err)
		}

		s.router.Handle(supportedUri.In, s.handleEndpoint(endpointProxyCfg))
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.Logger.Fatal(err)
	}
}

// Shutdown the http server gracefully
func (s *server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *server) buildEndpointProxyConfig(uriMap config.UriMap) (*endpointProxyConfig, error) {
	target, err := url.Parse(s.BaseEndpoint)
	if err != nil {
		return nil, err
	}

	endpointProxyCfg := &endpointProxyConfig{
		baseEndpoint:    target,
		UriMap:          uriMap,
		RequestResponse: s.Config.Overrides.Global,
	}

	uriCfg, ok := s.Config.Overrides.Uris[uriMap.In]
	if !ok {
		return endpointProxyCfg, nil
	}

	endpointProxyCfg.RequestResponse = mergeRequestResponse(s.Config.Overrides.Global, uriCfg)
	return endpointProxyCfg, nil
}

// Merge two config.RequestResponse structs, extending header lists and merging bodies.
func mergeRequestResponse(a, b config.RequestResponse) config.RequestResponse {
	return config.RequestResponse{
		Request:  mergeOverrideConfig(a.Request, b.Request),
		Response: mergeOverrideConfig(a.Response, b.Response),
	}
}

func mergeOverrideConfig(a, b config.OverrideConfig) config.OverrideConfig {
	return config.OverrideConfig{
		Headers: append(copyHeadersSlice(a.Headers), copyHeadersSlice(b.Headers)...),
		Body:    mergeBody(a.Body, b.Body),
	}
}

func copyHeadersSlice(headers []config.Header) []config.Header {
	copied := make([]config.Header, len(headers))

	for i, h := range headers {
		copied[i] = copyHeader(h)
	}

	return copied
}

func copyHeader(h config.Header) config.Header {
	return config.Header{
		Operation: h.Operation,
		Name:      h.Name,
		Text:      h.Text,
		File:      h.File,
		Request: config.Request{
			Method: h.Request.Method,
			Url:    h.Request.Url,
			Response: config.ReqResponse{
				ResultPath: h.Request.Response.ResultPath,
			},
			JsonBody: h.Request.JsonBody,
		},
	}
}

func mergeBody(a, b config.Body) config.Body {
	// If b.Template is set, use it; otherwise use a.Template
	template := a.Template

	if b.Template != "" {
		template = b.Template
	}

	// Extend patches
	return config.Body{
		Patches:  append(copyPatchesSlice(a.Patches), copyPatchesSlice(b.Patches)...),
		Template: template,
	}
}

func copyPatchesSlice(patches []config.Patch) []config.Patch {
	copied := make([]config.Patch, len(patches))

	for i, p := range patches {
		copied[i] = copyPatch(p)
	}

	return copied
}

func copyPatch(p config.Patch) config.Patch {
	return config.Patch{
		Operation: p.Operation,
		Path:      p.Path,
		Value:     p.Value,
	}
}
