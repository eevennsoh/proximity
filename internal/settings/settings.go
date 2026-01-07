package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Struct struct {
	AutoStartProxy bool           `yaml:"autoStartProxy" toml:"autoStartProxy"`
	Vars           map[string]any `yaml:"vars" toml:"vars"`
}

var defaultSettings Struct = Struct{
	AutoStartProxy: false,
	Vars:           make(map[string]any),
}

func Read(settingsPath string) (*Struct, error) {
	home := os.Getenv("HOME")
	basePath := filepath.Join(home, settingsPath)

	extensions := []string{".json", ".yaml", ".yml", ".toml"}

	for _, ext := range extensions {
		fullPath := basePath + ext

		bytes, err := os.ReadFile(fullPath)

		if errors.Is(err, os.ErrNotExist) {
			continue // Try next extension
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read settings: %v", err)
		}

		// Parse based on extension
		var settings Struct

		switch ext {
		case ".json":
			err = json.Unmarshal(bytes, &settings)
		case ".yaml", ".yml":
			err = yaml.Unmarshal(bytes, &settings)
		case ".toml":
			err = toml.Unmarshal(bytes, &settings)
		}

		if err != nil {
			return nil, err
		}

		return &settings, nil
	}

	// No file found
	return &defaultSettings, nil
}
