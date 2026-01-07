package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/template"

	"github.com/go-chi/chi"
)

type server struct {
	Options

	router     *chi.Mux
	httpServer *http.Server

	renderer *template.Renderer
}

func New(options Options) Interface {
	router := chi.NewRouter()

	httpServer := &http.Server{
		Addr:    fmt.Sprint(":", options.Port),
		Handler: router,
	}

	return &server{
		Options:    options,
		router:     router,
		httpServer: httpServer,
		renderer:   template.NewRenderer(options.Logger),
	}
}

func (s *server) RunServer(ctx context.Context) {
	s.Logger.Printf("starting http server on port %d", s.Options.Port)

	// Log out all requests coming in
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Logger.Println(r.Method, r.URL.Path, r.Header.Get("User-Agent"))
			next.ServeHTTP(w, r)
		})
	})

	combinedUriConfigs, err := s.combineCommonUriConfigs()
	if err != nil {
		s.Logger.Fatal(err)
	}

	for uri, endpointProxyCfgMap := range combinedUriConfigs {
		for method, cfg := range endpointProxyCfgMap {
			s.router.Method(method, uri, s.handleEndpoint(cfg))
		}
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.Logger.Fatal(err)
	}
}

// Shutdown the http server gracefully
func (s *server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *server) combineCommonUriConfigs() (map[string]map[string]*endpointProxyConfig, error) {
	combinedUriConfigs := make(map[string]map[string]*endpointProxyConfig)

	for _, uriGroup := range s.UriGroups {
		for _, supportedUri := range uriGroup.SupportedUris {
			endpointProxyCfgMap, err := s.buildEndpointProxyConfigs(supportedUri)
			if err != nil {
				return nil, err
			}

			existingEndpointProxyCfgMap, ok := combinedUriConfigs[supportedUri.In]
			if !ok {
				combinedUriConfigs[supportedUri.In] = endpointProxyCfgMap
				continue
			}

			for httpMethod, endpointProxyCfg := range endpointProxyCfgMap {
				if _, ok := existingEndpointProxyCfgMap[httpMethod]; ok {
					return nil, fmt.Errorf("route %s has multiple uris configs mapped to the http method \"%s\"",
						supportedUri.In, httpMethod,
					)
				}

				existingEndpointProxyCfgMap[httpMethod] = endpointProxyCfg
			}
		}
	}

	return combinedUriConfigs, nil
}

func (s *server) buildEndpointProxyConfigs(uriMap config.UriMap) (map[string]*endpointProxyConfig, error) {
	endpointProxyConfigMap := make(map[string]*endpointProxyConfig)

	baseEndpoint, err := s.getBaseEndpoint(uriMap)
	if err != nil {
		return nil, err
	}

	target, err := url.Parse(baseEndpoint)
	if err != nil {
		return nil, err
	}

	for _, outMethod := range uriMap.Out {
		httpMethod := outMethod.Method

		endpointProxyCfg := &endpointProxyConfig{
			baseEndpoint:    target,
			UriMap:          uriMap,
			Out:             outMethod,
			RequestResponse: s.Overrides.Global,
		}

		uriCfgMap, ok := s.Overrides.Uris[uriMap.In]
		if !ok {
			endpointProxyConfigMap[httpMethod] = endpointProxyCfg
			continue
		}

		if reqResp, ok := uriCfgMap[httpMethod]; ok {
			endpointProxyCfg.RequestResponse = mergeRequestResponse(s.Overrides.Global, reqResp)
		}

		endpointProxyConfigMap[httpMethod] = endpointProxyCfg
	}

	return endpointProxyConfigMap, nil
}

func (s *server) getBaseEndpoint(uriMap config.UriMap) (string, error) {
	if uriMap.BaseEndpoint != "" {
		return uriMap.BaseEndpoint, nil
	}

	env := map[string]any{
		"settings": s.Settings.Vars,
	}

	baseEndpointBytes, err := s.renderer.RenderExpr(s.BaseEndpoint, env, nil)

	if err != nil {
		return "", fmt.Errorf("failed to evaluate baseEndpoint expr: %w", err)
	}

	return string(baseEndpointBytes), nil
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
		StatusCode: b.StatusCode,
		Headers:    append(copyHeadersSlice(a.Headers), copyHeadersSlice(b.Headers)...),
		Body:       mergeBody(a.Body, b.Body),
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
		Input: config.Input{
			Text:     h.Text,
			File:     h.File,
			Template: h.Template,
			Expr:     h.Expr,
			Request: config.Request{
				Method: h.Request.Method,
				Url:    h.Request.Url,
				Response: config.ReqResponse{
					ResultPath: h.Request.Response.ResultPath,
				},
				JsonBody: h.Request.JsonBody,
			},
		},
	}
}

func mergeBody(a, b config.Body) config.Body {
	// If b.Template is set, use it; otherwise use a.Template
	template := a.Template

	if b.Template != "" {
		template = b.Template
	}

	// Same for text
	text := a.Text

	if b.Text != "" {
		text = b.Text
	}

	// Same for expr
	expr := a.Expr

	if b.Expr != "" {
		expr = b.Expr
	}

	// Extend patches
	return config.Body{
		Patches:  append(copyPatchesSlice(a.Patches), copyPatchesSlice(b.Patches)...),
		Text:     text,
		Template: template,
		Expr:     expr,
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
