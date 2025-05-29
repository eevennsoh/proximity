package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"

	"github.com/go-chi/chi"
	"github.com/jinzhu/copier"
)

type server struct {
	Options
	*config.Config

	router     *chi.Mux
	httpServer *http.Server
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
	}
}

func (s *server) RunServer(ctx context.Context) {
	log.Printf("starting http server on port %d", s.Options.Port)

	s.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	for _, supportedUri := range s.SupportedUris {
		endpointProxyCfg, err := s.buildEndpointProxyConfig(supportedUri)
		if err != nil {
			log.Fatal(err)
		}

		s.router.Handle(supportedUri.In, s.endpointProxy(endpointProxyCfg))
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
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
		baseEndpoint: target,
		UriMap:       uriMap,
	}

	err = copier.CopyWithOption(&endpointProxyCfg.OverrideConfig, &s.Config.Overrides.Global, copier.Option{
		IgnoreEmpty: true,
		DeepCopy:    true,
	})

	if err != nil {
		return nil, err
	}

	uriCfg, ok := s.Config.Overrides.Uris[uriMap.In]

	if !ok {
		return endpointProxyCfg, nil
	}

	err = copier.CopyWithOption(&endpointProxyCfg.OverrideConfig, &uriCfg, copier.Option{
		IgnoreEmpty: true,
		DeepCopy:    true,
	})

	if err != nil {
		return nil, err
	}

	return endpointProxyCfg, nil
}
