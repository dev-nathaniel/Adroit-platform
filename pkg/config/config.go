package config

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
	"time"
)

func init() {
	LoadDotEnv()
}

// LoadDotEnv loads key-value pairs from a .env file into the process environment
func LoadDotEnv(filenames ...string) error {
	if len(filenames) == 0 {
		filenames = []string{".env", "../.env", "../../.env"}
	}

	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			// Strip surrounding quotes if present
			if len(val) >= 2 {
				if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
					val = val[1 : len(val)-1]
				}
			}

			// Set in environment if not already defined or empty
			if current := os.Getenv(key); current == "" {
				_ = os.Setenv(key, val)
			}
		}
		return nil
	}
	return nil
}

// GetEnv retrieves an environment variable with a fallback default value
func GetEnv(key, defaultValue string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultValue
}

// GetEnvInt retrieves an integer environment variable with a fallback default value
func GetEnvInt(key string, defaultValue int) int {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// GetEnvBool retrieves a boolean environment variable with a fallback default value
func GetEnvBool(key string, defaultValue bool) bool {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// GetEnvDuration retrieves a time.Duration environment variable with a fallback
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		if durVal, err := time.ParseDuration(val); err == nil {
			return durVal
		}
	}
	return defaultValue
}

// BaseConfig contains common settings shared by all microservices
type BaseConfig struct {
	Env       string
	Port      int
	LogLevel  string
	LogFormat string
}

// LoadBaseConfig loads common platform environment variables
func LoadBaseConfig(defaultPort int) BaseConfig {
	return BaseConfig{
		Env:       GetEnv("ADROIT_ENV", "development"),
		Port:      GetEnvInt("PORT", defaultPort),
		LogLevel:  GetEnv("ADROIT_LOG_LEVEL", "debug"),
		LogFormat: GetEnv("ADROIT_LOG_FORMAT", "json"),
	}
}
