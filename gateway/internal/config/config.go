package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	JWT      JWTConfig      `yaml:"jwt"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Database DatabaseConfig `yaml:"database"`
	Sync     SyncConfig     `yaml:"sync"`
	DemoUsers []DemoUser    `yaml:"demo_users"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type SyncConfig struct {
	DataDir  string        `yaml:"data_dir"`
	Interval time.Duration `yaml:"interval"`
}

// DSN 返回 keyword=value 形式连接串（密码含特殊字符无需转义）
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		d.Host, d.Port, d.User, d.Password, d.Name)
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"`
}

type JWTConfig struct {
	Secret    string        `yaml:"secret"`
	AccessTTL time.Duration `yaml:"access_ttl"`
}

type DefaultsConfig struct {
	TenantID string `yaml:"tenant_id"`
	OrgID    string `yaml:"org_id"`
}

type DemoUser struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	UserID   string `yaml:"user_id"`
	Role     string `yaml:"role"`
	DeptID   string `yaml:"dept_id"`
}

var current *Config

func MustLoad() *Config {
	path := os.Getenv("GATEWAY_CONFIG")
	if path == "" {
		path = "etc/config.yaml"
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("config: read %s: %v", path, err))
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		panic(fmt.Sprintf("config: parse %s: %v", path, err))
	}
	if cfg.JWT.Secret == "" || cfg.JWT.AccessTTL <= 0 || cfg.Server.Addr == "" {
		panic(fmt.Sprintf("config: jwt.secret / jwt.access_ttl / server.addr are required in %s", path))
	}
	if cfg.Database.Name == "" {
		panic(fmt.Sprintf("config: database.* are required in %s", path))
	}
	if cfg.Sync.DataDir == "" || cfg.Sync.Interval <= 0 {
		panic(fmt.Sprintf("config: sync.data_dir / sync.interval are required in %s", path))
	}

	current = cfg
	return cfg
}

func Get() *Config {
	if current == nil {
		panic("config: not loaded")
	}
	return current
}
