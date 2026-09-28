package config

import (
	"os"
	"strconv"
	"time"
)

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
