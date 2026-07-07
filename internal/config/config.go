package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Defaults struct {
	HTTPAddr string
}

type Config struct {
	ServiceName       string
	Env               string
	HTTPAddr          string
	LogLevel          string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	DatabaseURL       string
	RedisAddr         string
	TemporalAddress   string
	MinIOEndpoint     string
	JaegerEndpoint    string
	RequestIDHeader   string
}

func Load(serviceName string, defaults Defaults) (Config, error) {
	if strings.TrimSpace(serviceName) == "" {
		return Config{}, errors.New("serviceName 不能为空")
	}

	configFile := strings.TrimSpace(os.Getenv("APP_CONFIG_FILE"))
	if configFile == "" {
		configFile = "config/local.env"
	}
	if err := loadEnvFileIfExists(configFile); err != nil {
		return Config{}, err
	}

	prefix := envPrefix(serviceName)
	httpAddr := scopedEnv(prefix, "HTTP_ADDR", defaults.HTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	readTimeout, err := durationEnv(prefix, "READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	readHeaderTimeout, err := durationEnv(prefix, "READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationEnv(prefix, "WRITE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationEnv(prefix, "IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationEnv(prefix, "SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ServiceName:       scopedEnv(prefix, "SERVICE_NAME", serviceName),
		Env:               env("APP_ENV", "local"),
		HTTPAddr:          httpAddr,
		LogLevel:          scopedEnv(prefix, "LOG_LEVEL", "info"),
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		DatabaseURL:       env("DATABASE_URL", "postgres://agent_runtime:agent_runtime@localhost:5432/agent_runtime?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		TemporalAddress:   env("TEMPORAL_ADDRESS", "localhost:7233"),
		MinIOEndpoint:     env("MINIO_ENDPOINT", "localhost:9000"),
		JaegerEndpoint:    env("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
		RequestIDHeader:   scopedEnv(prefix, "REQUEST_ID_HEADER", "X-Request-ID"),
	}, nil
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func scopedEnv(prefix string, key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(prefix + "_" + key)); value != "" {
		return value
	}
	return env(key, fallback)
}

func durationEnv(prefix string, key string, fallback time.Duration) (time.Duration, error) {
	raw := scopedEnv(prefix, key, "")
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("配置 %s 不是合法 duration: %w", key, err)
	}
	return value, nil
}

func envPrefix(serviceName string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_")
	return strings.ToUpper(replacer.Replace(serviceName))
}

func loadEnvFileIfExists(path string) error {
	if path == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("配置文件 %s 存在非法行: %s", path, line)
		}
		key = strings.TrimSpace(key)
		value = trimEnvValue(strings.TrimSpace(value))
		if key == "" {
			return fmt.Errorf("配置文件 %s 存在空 key", path)
		}
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("设置配置 %s 失败: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}
	return nil
}

func trimEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}
	if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
		(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
		return value[1 : len(value)-1]
	}
	return value
}
