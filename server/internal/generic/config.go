package generic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

// 认证方式类型
const (
	AuthTypeAPIKey     = "api_key"     // 请求头携带 API Key
	AuthTypeBearer     = "bearer"      // 请求头携带 Bearer Token
	AuthTypeCookie     = "cookie"      // 请求头携带 Cookie
	AuthTypeOAuthLocal = "oauth_local" // 读取本机登录态文件
	AuthTypeNone       = "none"        // 无需认证
)

// 数据源类型
const (
	SourceQuota   = "quota"   // 配额剩余
	SourceBalance = "balance" // 余额
	SourceCost    = "cost"    // 费用
	SourceTokens  = "tokens"  // token 消耗
)

// PlatformConfig 平台适配器配置（配置驱动，零代码接入新平台）
type PlatformConfig struct {
	Name        string        `json:"name"`         // 平台标识（唯一）
	DisplayName string        `json:"display_name"` // 显示名称
	Enabled     bool          `json:"enabled"`      // 是否启用
	Interval    int           `json:"interval"`     // 轮询间隔（秒），默认 300
	Auth        AuthConfig    `json:"auth"`         // 认证配置
	Sources     []SourceConfig `json:"sources"`     // 数据源列表
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type    string `json:"type"`     // api_key | bearer | cookie | oauth_local | none
	Header  string `json:"header"`   // 请求头名称，默认 Authorization
	Key     string `json:"key"`      // 凭证值（api_key/bearer/cookie 直接填）
	KeyFrom string `json:"key_from"` // 从本机读取：env:VAR 或 file:path:jsonpath
}

// SourceConfig 数据源配置
type SourceConfig struct {
	Name     string            `json:"name"`     // quota | balance | cost | tokens
	URL      string            `json:"url"`      // 接口地址
	Method   string            `json:"method"`   // GET（默认）| POST
	Interval int               `json:"interval"` // 覆盖平台级轮询间隔（秒）
	Mapping  map[string]string `json:"mapping"`  // 统一字段 -> JSONPath 表达式
	Static   map[string]string `json:"static"`   // 统一字段 -> 静态值
}

// ConfigStore 配置存储接口（由 store 实现）
type ConfigStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// settingsKey 通用适配器配置在 settings 表中的存储键
const settingsKey = "generic_platforms"

// LoadPlatforms 从存储加载平台配置列表
func LoadPlatforms(s ConfigStore) ([]PlatformConfig, error) {
	raw, err := s.GetSetting(settingsKey)
	if err != nil || raw == "" {
		return nil, nil
	}
	var platforms []PlatformConfig
	if err := json.Unmarshal([]byte(raw), &platforms); err != nil {
		return nil, fmt.Errorf("generic: 解析平台配置失败: %w", err)
	}
	return platforms, nil
}

// SavePlatforms 保存平台配置列表到存储
func SavePlatforms(s ConfigStore, platforms []PlatformConfig) error {
	data, err := json.MarshalIndent(platforms, "", "  ")
	if err != nil {
		return fmt.Errorf("generic: 序列化平台配置失败: %w", err)
	}
	return s.SetSetting(settingsKey, string(data))
}

// FindPlatform 按名称查找平台
func FindPlatform(platforms []PlatformConfig, name string) *PlatformConfig {
	for i := range platforms {
		if platforms[i].Name == name {
			return &platforms[i]
		}
	}
	return nil
}

// ResolveKey 解析凭证来源：
//   - 直接值（Key 非空）
//   - env:VAR 从环境变量读取
//   - file:path:jsonpath 从本机文件读取（oauth_local 场景）
func (a AuthConfig) ResolveKey() (string, error) {
	if a.Key != "" {
		return a.Key, nil
	}
	if a.KeyFrom == "" {
		return "", nil
	}

	switch {
	case strings.HasPrefix(a.KeyFrom, "env:"):
		envVar := strings.TrimPrefix(a.KeyFrom, "env:")
		val := os.Getenv(envVar)
		if val == "" {
			return "", fmt.Errorf("generic: 环境变量 %s 未设置", envVar)
		}
		return val, nil

	case strings.HasPrefix(a.KeyFrom, "file:"):
		// 格式: file:path:jsonpath
		parts := strings.SplitN(a.KeyFrom, ":", 3)
		if len(parts) < 3 {
			return "", fmt.Errorf("generic: file: 格式应为 file:path:jsonpath，实际: %s", a.KeyFrom)
		}
		path := parts[1]
		expr := parts[2]
		if !filepath.IsAbs(path) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("generic: 获取用户目录失败: %w", err)
			}
			path = filepath.Join(home, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("generic: 读取本机登录态文件失败 %s: %w", path, err)
		}
		var doc interface{}
		if err := json.Unmarshal(data, &doc); err != nil {
			return "", fmt.Errorf("generic: 解析登录态文件失败 %s: %w", path, err)
		}
		val, err := jsonpath.Get(expr, doc)
		if err != nil {
			return "", fmt.Errorf("generic: 从登录态文件提取字段失败 %s: %w", expr, err)
		}
		return fmt.Sprintf("%v", val), nil

	default:
		return "", fmt.Errorf("generic: 不支持的凭证来源格式: %s", a.KeyFrom)
	}
}

// EffectiveInterval 返回数据源生效的轮询间隔
func (s SourceConfig) EffectiveInterval(platformInterval int) int {
	if s.Interval > 0 {
		return s.Interval
	}
	if platformInterval > 0 {
		return platformInterval
	}
	return 300 // 默认 5 分钟
}