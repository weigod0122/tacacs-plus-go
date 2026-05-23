package cfg

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"tacacs/pkg/public/apolloConfig"

	"gopkg.in/yaml.v3"
)

var (
	lock         = new(sync.RWMutex)
	serverConfig *ServerGlobalConfig
	clientConfig *ClientGlobalConfig
	swmConfig    *SwmGlobalConfig
)

type DatabaseInfo struct {
	Address  string `yaml:"address"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Table    string `yaml:"table"`
}

type SwmAuthConfig struct {
	Enforce          bool     `yaml:"enforce"`
	SharedSecret     string   `yaml:"shared_secret"`
	SharedSecretEnv  string   `yaml:"shared_secret_env"`
	SharedSecretFile string   `yaml:"shared_secret_file"`
	MaxSkewSeconds   int      `yaml:"max_skew_seconds"`
	AllowedOrigin    string   `yaml:"allowed_origin"`
	AllowedCIDRs     []string `yaml:"allowed_cidrs"`
}

func (s *SwmAuthConfig) ResolveSecret() string {
	if s == nil {
		return ""
	}
	if s.SharedSecretEnv != "" {
		if v := os.Getenv(s.SharedSecretEnv); v != "" {
			return v
		}
	}
	if s.SharedSecretFile != "" {
		if b, err := os.ReadFile(s.SharedSecretFile); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return s.SharedSecret
}

// defaultAllowedCIDRs 是未配置 allowed_cidrs 时的兜底白名单。
// 默认仅放行 loopback,匹配最常见的「SwM 与 Server 同机部署」拓扑。
// 跨机部署需在配置文件显式声明 allowed_cidrs。
var defaultAllowedCIDRs = []string{"127.0.0.1/32", "::1/128"}

// ResolveAllowedNets 把配置里的字符串 CIDR 列表解析成 *net.IPNet。
// 列表为空时返回 defaultAllowedCIDRs;任一条目格式非法返回 error。
func (s *SwmAuthConfig) ResolveAllowedNets() ([]*net.IPNet, error) {
	src := s.AllowedCIDRs
	if len(src) == 0 {
		src = defaultAllowedCIDRs
	}
	nets := make([]*net.IPNet, 0, len(src))
	for _, c := range src {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w (single IPs must include mask, e.g. 10.0.0.1/32)", c, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

type FeishuConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppId        string `yaml:"app_id"`
	AppIdEnv     string `yaml:"app_id_env"`
	AppSecret    string `yaml:"app_secret"`
	AppSecretEnv string `yaml:"app_secret_env"`
}

func resolveEnvOr(envName, fallback string) string {
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return fallback
}

func (f *FeishuConfig) ResolveAppId() string {
	if f == nil {
		return ""
	}
	return resolveEnvOr(f.AppIdEnv, f.AppId)
}

func (f *FeishuConfig) ResolveAppSecret() string {
	if f == nil {
		return ""
	}
	return resolveEnvOr(f.AppSecretEnv, f.AppSecret)
}

type ServerGlobalConfig struct {
	LogFilePath string                             `yaml:"log_file_path"`
	Database    map[string]map[string]DatabaseInfo `yaml:"database"`
	Http        string                             `yaml:"http"`
	Manager     string                             `yaml:"manager"`
	SwmAuth     SwmAuthConfig                      `yaml:"swm_auth"`
	Feishu      FeishuConfig                       `yaml:"feishu"`
}

type ClientGlobalConfig struct {
	LogFilePath string                             `yaml:"log_file_path"`
	Database    map[string]map[string]DatabaseInfo `yaml:"database"`
	Http        string                             `yaml:"http"`
	Manager     string                             `yaml:"manager"`
	TacPlus     map[string]string                  `yaml:"tacPlus"`
	Feishu      FeishuConfig                       `yaml:"feishu"`
}

// SwmGlobalConfig 是 swm 前端 + 反代进程的配置。
// 与 server/client 不同，swm 没有 DB；仅靠 HTTP 调 tacacs-server。
type SwmGlobalConfig struct {
	LogFilePath      string `yaml:"log_file_path"`
	Http             string `yaml:"http"`
	Manager          string `yaml:"manager"`
	TacacsManagerUrl string `yaml:"tacacs_manager_url"`
	SessionTimeOut   int    `yaml:"session_time_out"`

	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`

	SwmSharedSecret     string `yaml:"swm_shared_secret"`
	SwmSharedSecretEnv  string `yaml:"swm_shared_secret_env"`
	SwmSharedSecretFile string `yaml:"swm_shared_secret_file"`

	Feishu FeishuConfig `yaml:"feishu"`
}

// ResolveSwmSecret 解析与 tacacs-server 共享的 HMAC 密钥，优先级 env > file > 明文。
func (g *SwmGlobalConfig) ResolveSwmSecret() string {
	if g == nil {
		return ""
	}
	if g.SwmSharedSecretEnv != "" {
		if v := os.Getenv(g.SwmSharedSecretEnv); v != "" {
			return v
		}
	}
	if g.SwmSharedSecretFile != "" {
		if b, err := os.ReadFile(g.SwmSharedSecretFile); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return g.SwmSharedSecret
}

func ServerConfig() *ServerGlobalConfig {
	lock.RLock()
	defer lock.RUnlock()
	return serverConfig
}

// SetServerConfig 替换全局 serverConfig。仅供测试 / 配置热重载使用。
// 生产代码应通过 ParseConfig 加载。
func SetServerConfig(c *ServerGlobalConfig) {
	lock.Lock()
	defer lock.Unlock()
	serverConfig = c
}

func ClientConfig() *ClientGlobalConfig {
	lock.RLock()
	defer lock.RUnlock()
	return clientConfig
}

func SwmConfig() *SwmGlobalConfig {
	lock.RLock()
	defer lock.RUnlock()
	return swmConfig
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
func ToString(filePath string) (string, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ToTrimString(filePath string) (string, error) {
	str, err := ToString(filePath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(str), nil
}

func ParseConfig(item, cfg string) error {
	content, err := loadConfigContent(item, cfg)
	if err != nil {
		return err
	}

	lock.Lock()
	defer lock.Unlock()

	switch item {
	case "server":
		var c ServerGlobalConfig
		if err := yaml.Unmarshal([]byte(content), &c); err != nil {
			return fmt.Errorf("parse %s config fail: %v", item, err)
		}
		serverConfig = &c
	case "client":
		var c ClientGlobalConfig
		if err := yaml.Unmarshal([]byte(content), &c); err != nil {
			return fmt.Errorf("parse %s config fail: %v", item, err)
		}
		clientConfig = &c
	case "swm":
		var c SwmGlobalConfig
		if err := yaml.Unmarshal([]byte(content), &c); err != nil {
			return fmt.Errorf("parse %s config fail: %v", item, err)
		}
		swmConfig = &c
	default:
		return fmt.Errorf("unknown config item: %s", item)
	}
	return nil
}

func loadConfigContent(item, cfg string) (string, error) {
	content, err := apolloConfig.GetConfig(item)
	if err == nil {
		return content, nil
	}

	if cfg == "" {
		return "", fmt.Errorf("apollo unavailable (%v) and no config file specified, use -c to specify one", err)
	}
	if exists, _ := PathExists(cfg); !exists {
		return "", fmt.Errorf("config file: %s not found", cfg)
	}
	return ToTrimString(cfg)
}
