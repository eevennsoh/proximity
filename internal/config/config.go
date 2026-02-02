package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Operation string

const (
	AddOperation    Operation = "add"
	RemoveOperation Operation = "remove"
)

type Config struct {
	BaseEndpoint string     `yaml:"baseEndpoint"`
	UriGroups    []UriGroup `yaml:"uriGroups"`
	Overrides    Overrides  `yaml:"overrides"`
}

type UriGroup struct {
	Name          string   `yaml:"name" json:"name"`
	Hidden        bool     `yaml:"hidden" json:"hidden,omitempty"`
	SupportedUris []UriMap `yaml:"supportedUris" json:"supportedUris"`
}

type UriMap struct {
	In           string      `yaml:"in" json:"in"`
	Description  string      `yaml:"description" json:"description,omitempty"`
	Out          []OutMethod `yaml:"out" json:"out,omitempty"`
	BaseEndpoint string      `yaml:"baseEndpoint" json:"baseEndpoint,omitempty"`
}

type Forward struct {
	Path    Input    `yaml:"path"`
	Headers []Header `yaml:"headers"`
}

// FetchRequest defines a single HTTP request to make
type FetchRequest struct {
	Method  string   `yaml:"method"`
	Url     Input    `yaml:"url"`
	Headers []Header `yaml:"headers"`
	Body    Input    `yaml:"body"`
	Timeout string   `yaml:"timeout"`
}

type Fetch struct {
	Requests map[string]FetchRequest `yaml:"requests"`
}

type StatusCodeInput struct {
	Int  int    `yaml:"int"`
	Expr string `yaml:"expr"`
}

type OutMethod struct {
	Method string `yaml:"method" json:"method"`
	Input  `yaml:",inline"`
}

type Overrides struct {
	Global RequestResponse `yaml:"global"`

	// First layer is route
	// Second layer is http method
	Uris map[string]map[string]RequestResponse `yaml:"uris"`
}

type RequestResponse struct {
	Forward  *Forward       `yaml:"forward,omitempty"`
	Fetch    *Fetch         `yaml:"fetch,omitempty"`
	Request  OverrideConfig `yaml:"request,omitempty"`
	Response OverrideConfig `yaml:"response,omitempty"`
}

type OverrideConfig struct {
	// StatusCode is only used if uriMap.Out is not defined, otherwise it forwards the upstream response status code
	StatusCode StatusCodeInput `yaml:"statusCode"`
	Headers    []Header        `yaml:"headers"`
	Body       Body            `yaml:"body"`
}

type Input struct {
	Text     string  `yaml:"text"`
	Template string  `yaml:"template"`
	Expr     string  `yaml:"expr"`
	File     string  `yaml:"file"`
	Request  Request `yaml:"request"`
}

// IsEmpty returns true if the Input has no value set
func (i Input) IsEmpty() bool {
	return i.Text == "" && i.Template == "" && i.Expr == "" && i.File == "" && i.Request.Url == ""
}

type Header struct {
	Operation Operation `yaml:"op"`
	Name      string    `yaml:"name"`
	Input     `yaml:",inline"`
}

type Request struct {
	Method   string      `yaml:"method"`
	Url      string      `yaml:"url"`
	Response ReqResponse `yaml:"response"`
	JsonBody string      `yaml:"jsonBody"`
}

type ReqResponse struct {
	ResultPath string `yaml:"resultPath"`
}

type Body struct {
	Patches  []Patch `yaml:"patches"`
	Text     string  `yaml:"text"`
	Template string  `yaml:"template"`
	Expr     string  `yaml:"expr"`
}

type Patch struct {
	Operation string `json:"op" yaml:"op"`
	Path      string `json:"path" yaml:"path"`
	Value     string `json:"value" yaml:"value"`
}

func ReadConfig(configData string) (*Config, error) {
	decodedConfig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(configData))
	if err != nil {
		return nil, err
	}

	return LoadFromBytes(decodedConfig)
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return LoadFromBytes(data)
}

func LoadFromBytes(data []byte) (*Config, error) {
	var config Config

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}
