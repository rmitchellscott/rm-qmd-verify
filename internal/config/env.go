package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func Get(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if path := os.Getenv(key + "_FILE"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return defaultValue
}

func GetInt(key string, defaultValue int) int {
	val := Get(key, "")
	if val == "" {
		return defaultValue
	}
	if intVal, err := strconv.Atoi(val); err == nil {
		return intVal
	}
	return defaultValue
}

func GetBool(key string, defaultValue bool) bool {
	val := strings.ToLower(Get(key, ""))
	if val == "" {
		return defaultValue
	}
	return val == "true" || val == "1" || val == "yes"
}

func GetDuration(key string, defaultValue time.Duration) time.Duration {
	val := Get(key, "")
	if val == "" {
		return defaultValue
	}
	if duration, err := time.ParseDuration(val); err == nil {
		return duration
	}
	return defaultValue
}
