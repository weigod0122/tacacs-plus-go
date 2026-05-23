package apolloConfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/apolloconfig/agollo/v5"
	"github.com/apolloconfig/agollo/v5/env/config"
	"github.com/apolloconfig/agollo/v5/storage"
	"gopkg.in/yaml.v3"
)

var (
	apolloClient    agollo.Client
	apolloAppConfig *config.AppConfig
)

type fileConfig struct {
	AppID     string `yaml:"app_id"`
	Cluster   string `yaml:"cluster"`
	IP        string `yaml:"ip"`
	Namespace string `yaml:"namespace"`
	Secret    string `yaml:"secret"`
}

func Init() error {
	cfg := resolveConfig()

	var missing []string
	if cfg.AppID == "" {
		missing = append(missing, "app_id")
	}
	if cfg.IP == "" {
		missing = append(missing, "ip")
	}
	if cfg.Secret == "" {
		missing = append(missing, "secret")
	}
	if len(missing) > 0 {
		return fmt.Errorf("apollo config missing required fields: %s (set via env or apollo.yaml)", strings.Join(missing, ", "))
	}

	apolloAppConfig = &config.AppConfig{
		AppID:          cfg.AppID,
		Cluster:        cfg.Cluster,
		IP:             cfg.IP,
		NamespaceName:  cfg.Namespace,
		IsBackupConfig: false,
		Secret:         cfg.Secret,
		MustStart:      true,
	}

	client, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return apolloAppConfig, nil
	})
	if err != nil {
		return fmt.Errorf("start apollo error: %s", err)
	}
	apolloClient = client
	return nil
}

func IsBeSet() bool {
	return apolloClient != nil
}

func GetConfig(key string) (string, error) {
	if apolloClient == nil {
		return "", fmt.Errorf("apollo client not initialized")
	}
	cache := apolloClient.GetConfigCache(apolloAppConfig.NamespaceName)
	value, err := cache.Get(key)
	if err != nil {
		return "", fmt.Errorf("get key(%v)`s config error: %s", key, err)
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("key(%v) value is not string", key)
	}
	return s, nil
}

func AddChangeListener(listener storage.ChangeListener) {
	if apolloClient != nil {
		apolloClient.AddChangeListener(listener)
	}
}

func resolveConfig() fileConfig {
	var fc fileConfig
	if data, err := os.ReadFile("apollo.yaml"); err == nil {
		_ = yaml.Unmarshal(data, &fc)
	}

	fc.AppID = envOr("APOLLO_APP_ID", fc.AppID)
	fc.Cluster = envOr("APOLLO_CLUSTER", fc.Cluster)
	fc.IP = envOr("APOLLO_IP", fc.IP)
	fc.Namespace = envOr("APOLLO_NAMESPACE", fc.Namespace)
	fc.Secret = envOr("APOLLO_SECRET", fc.Secret)

	if fc.Cluster == "" {
		fc.Cluster = "default"
	}
	if fc.Namespace == "" {
		fc.Namespace = "application"
	}

	return fc
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
