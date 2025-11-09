package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Struct struct {
	AutoStartProxy bool              `yaml:"autoStartProxy"`
	Vars           map[string]string `yaml:"vars"`
}

var defaultSettings Struct = Struct{
	AutoStartProxy: false,
	Vars:           make(map[string]string),
}

func Read(settingsPath string) (*Struct, error) {
	home := os.Getenv("HOME")
	fullPath := filepath.Join(home, settingsPath)

	bytes, err := os.ReadFile(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return &defaultSettings, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read settings: %v", err)
	}

	var settings Struct

	if err := json.Unmarshal(bytes, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}
