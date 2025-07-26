package ratelimit

import (
	"fmt"
	"time"
)

func getInt64FromResult(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("expected int64, got %T", value)
	}
}

func getInt64Config(config map[string]interface{}, key string) (int64, error) {
	value, exists := config[key]
	if !exists {
		return 0, fmt.Errorf("required config key '%s' not found", key)
	}

	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("config key '%s' must be a number, got %T", key, value)
	}
}

func getDurationConfig(config map[string]interface{}, key string) (time.Duration, error) {
	value, exists := config[key]
	if !exists {
		return 0, fmt.Errorf("required config key '%s' not found", key)
	}

	if duration, ok := value.(time.Duration); ok {
		return duration, nil
	}

	return 0, fmt.Errorf("config key '%s' must be a time.Duration, got %T", key, value)
}

func getStringConfig(config map[string]interface{}, key string) (string, error) {
	value, exists := config[key]
	if !exists {
		return "", fmt.Errorf("required config key '%s' not found", key)
	}

	if str, ok := value.(string); ok {
		return str, nil
	}

	return "", fmt.Errorf("config key '%s' must be a string, got %T", key, value)
}

func getIntConfig(config map[string]interface{}, key string) (int, error) {
	value, exists := config[key]
	if !exists {
		return 0, fmt.Errorf("required config key '%s' not found", key)
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("config key '%s' must be a number, got %T", key, value)
	}
}