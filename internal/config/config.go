package config

import (
	"encoding/base64"
	"strings"

	"gopkg.in/yaml.v3"
)

type Operation string

const (
	AddOperation    Operation = "add"
	RemoveOperation Operation = "remove"
)

type Config struct {
	BaseEndpoint  string    `yaml:"baseEndpoint"`
	SupportedUris []UriMap  `yaml:"supportedUris"`
	Overrides     Overrides `yaml:"overrides"`
}

type UriMap struct {
	In  string `yaml:"in"`
	Out string `yaml:"out"`
}

type Overrides struct {
	Global OverrideConfig            `yaml:"global"`
	Uris   map[string]OverrideConfig `yaml:"uris"`
}

type OverrideConfig struct {
	Headers  []Header `yaml:"headers"`
	Body     Body     `yaml:"body"`
	Response Response `yaml:"response"`
}

type Header struct {
	Operation Operation `yaml:"op"`
	Name      string    `yaml:"name"`
	Text      string    `yaml:"text"`
	File      string    `yaml:"file"`
	Request   Request   `yaml:"request"`
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
	Json []Patch `yaml:"application/json"`
	Text string  `yaml:"application/text"`
}

type Response struct {
	Json []Patch `yaml:"application/json"`
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

	var cfg Config

	if err := yaml.Unmarshal(decodedConfig, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
