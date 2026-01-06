// Package main provides the webadmin server entry point.
package main

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// WebAdminConfig represents the webadmin configuration.
type WebAdminConfig struct {
	Server  ServerConfig             `yaml:"server"`
	JukeBox JukeBoxConfig            `yaml:"19box"`
	Presets map[string]PresetConfig  `yaml:"presets"`
	Hooks   HooksConfig              `yaml:"hooks"`
}

// ServerConfig represents the webadmin server configuration.
type ServerConfig struct {
	Addr string `yaml:"addr"` // WebAdmin server address (default: ":8081")
}

// JukeBoxConfig represents the 19box-server configuration.
type JukeBoxConfig struct {
	Path       string `yaml:"path"`        // Path to 19box-server binary
	BaseConfig string `yaml:"base_config"` // Path to server.yaml
	EnvPath    string `yaml:"env_path"`    // Path to .env file
	AdminToken string `yaml:"admin_token"` // Admin API token
}

// PresetConfig represents a preset configuration.
type PresetConfig struct {
	Description string                 `yaml:"description"`
	Session     map[string]interface{} `yaml:"session"`
	Playlists   map[string]interface{} `yaml:"playlists"`
	Server      map[string]interface{} `yaml:"server"`
}

// HooksConfig represents lifecycle hooks configuration.
type HooksConfig struct {
	OnStart []ProcessConfig `yaml:"on_start"` // Processes to start with server
}

// ProcessConfig represents a child process configuration.
type ProcessConfig struct {
	Name    string   `yaml:"name"`    // Process name for logging
	Command string   `yaml:"command"` // Command to execute
	Args    []string `yaml:"args"`    // Command arguments
}

// BaseServerConfig represents the base server.yaml configuration.
// This is a simplified version for merging with presets.
type BaseServerConfig struct {
	Session   map[string]interface{} `yaml:"session"`
	Playlists map[string]interface{} `yaml:"playlists"`
	// Other fields are preserved as-is
	raw map[string]interface{}
}

// LoadWebAdminConfig loads the webadmin configuration from a YAML file.
func LoadWebAdminConfig(path string) (*WebAdminConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	var cfg WebAdminConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	// Set defaults
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8081"
	}

	// Environment variable overrides
	if env := os.Getenv("ADMIN_TOKEN"); env != "" {
		cfg.JukeBox.AdminToken = env
	}

	return &cfg, nil
}

// LoadBaseConfig loads the base server configuration.
func LoadBaseConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read base config file")
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse base config file")
	}

	return cfg, nil
}

// MergeConfig merges preset settings into base config.
func MergeConfig(base map[string]interface{}, preset PresetConfig) map[string]interface{} {
	result := deepCopy(base)

	// Merge session settings
	if preset.Session != nil {
		if session, ok := result["session"].(map[string]interface{}); ok {
			for k, v := range preset.Session {
				if v != nil {
					session[k] = v
				}
			}
		}
	}

	// Merge playlists settings
	if preset.Playlists != nil {
		var playlists map[string]interface{}
		if p, ok := result["playlists"].(map[string]interface{}); ok {
			playlists = p
		} else {
			playlists = make(map[string]interface{})
			result["playlists"] = playlists
		}

		for k, v := range preset.Playlists {
			if vm, ok := v.(map[string]interface{}); ok {
				if existing, ok := playlists[k].(map[string]interface{}); ok {
					for kk, vv := range vm {
						if vv != nil {
							existing[kk] = vv
						}
					}
				} else {
					playlists[k] = vm
				}
			}
		}
	}

	// Merge server settings (hooks, etc.)
	if preset.Server != nil {
		var server map[string]interface{}
		if s, ok := result["server"].(map[string]interface{}); ok {
			server = s
		} else {
			server = make(map[string]interface{})
			result["server"] = server
		}

		for k, v := range preset.Server {
			if vm, ok := v.(map[string]interface{}); ok {
				if existing, ok := server[k].(map[string]interface{}); ok {
					for kk, vv := range vm {
						if vv != nil {
							existing[kk] = vv
						}
					}
				} else {
					server[k] = vm
				}
			} else {
				server[k] = v
			}
		}
	}

	return result
}

// MergeFormData merges form data into config.
func MergeFormData(cfg map[string]interface{}, form map[string]interface{}) map[string]interface{} {
	result := deepCopy(cfg)

	// Merge session data
	if formSession, ok := form["session"].(map[string]interface{}); ok {
		var session map[string]interface{}
		if s, ok := result["session"].(map[string]interface{}); ok {
			session = s
		} else {
			session = make(map[string]interface{})
			result["session"] = session
		}

		for k, v := range formSession {
			if v != nil {
				session[k] = v
			}
		}
	}

	// Merge playlists data
	if formPlaylists, ok := form["playlists"].(map[string]interface{}); ok {
		var playlists map[string]interface{}
		if p, ok := result["playlists"].(map[string]interface{}); ok {
			playlists = p
		} else {
			playlists = make(map[string]interface{})
			result["playlists"] = playlists
		}

		for k, v := range formPlaylists {
			if vm, ok := v.(map[string]interface{}); ok {
				if existing, ok := playlists[k].(map[string]interface{}); ok {
					for kk, vv := range vm {
						if vv != nil && vv != "" {
							existing[kk] = vv
						}
					}
				} else {
					playlists[k] = vm
				}
			}
		}
	}

	// Merge server data (only hooks)
	if formServer, ok := form["server"].(map[string]interface{}); ok {
		if hooks, ok := formServer["hooks"]; ok {
			if server, ok := result["server"].(map[string]interface{}); ok {
				server["hooks"] = hooks
			} else {
				result["server"] = map[string]interface{}{"hooks": hooks}
			}
		}
	}

	return result
}

// SaveTempConfig saves config to a temporary file.
func SaveTempConfig(cfg map[string]interface{}) (string, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal config")
	}

	f, err := os.CreateTemp("", "19box-server-*.yaml")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp file")
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return "", errors.Wrap(err, "failed to write temp config")
	}

	return f.Name(), nil
}

// LoadEnvFile loads environment variables from a .env file.
func LoadEnvFile(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}

	envMap, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // .env is optional
		}
		return nil, errors.Wrap(err, "failed to read .env file")
	}

	var envVars []string
	for k, v := range envMap {
		envVars = append(envVars, k+"="+v)
	}

	return envVars, nil
}

// deepCopy creates a deep copy of a map.
func deepCopy(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			result[k] = deepCopy(vm)
		} else if vs, ok := v.([]interface{}); ok {
			newSlice := make([]interface{}, len(vs))
			copy(newSlice, vs)
			result[k] = newSlice
		} else {
			result[k] = v
		}
	}
	return result
}
