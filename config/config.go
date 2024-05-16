package config

import (
	"encoding/json"
	"os"
	"sync"
)

// AppConfig holds the structure of the configuration
type AppConfig struct {
	SelfPath  string `json:"selfpath"`
	Port      string `json:"port"`
	Blocksize int    `json:"blocksize"`
}

var (
	instance *AppConfig
	once     sync.Once
)

// LoadConfig initializes and returns the instance of AppConfig
func LoadConfig(filePath string) *AppConfig {
	once.Do(func() {
		instance = &AppConfig{
			SelfPath:  "http://www.example.com", // Default value
			Port:      "38870",                  // Default value
			Blocksize: 20,
		}
		// Load the config file if it exists, otherwise create one
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			saveConfig(filePath)
		} else {
			loadConfig(filePath)
		}
	})
	return instance
}

// loadConfig loads the settings from the file
func loadConfig(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(instance); err != nil {
		panic(err)
	}
}

// saveConfig saves the current settings to the file
func saveConfig(filePath string) {
	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(instance); err != nil {
		panic(err)
	}
}

// GetConfigValue returns the value of the configuration by key
func GetConfigValue(key string) interface{} {
	switch key {
	case "selfpath":
		return instance.SelfPath
	case "port":
		return instance.Port
	case "blocksize":
		return instance.Blocksize
	default:
		return ""
	}
}
