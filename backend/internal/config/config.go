package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	DB       DBConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Twilio   TwilioConfig
}

type ServerConfig struct {
	Port string
	Env  string
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
}

type JWTConfig struct {
	Secret     string
	AccessTTL  string
	RefreshTTL string
}

type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	VerifySID  string
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	viper.ReadInConfig() // не фатально если нет файла

	cfg := &Config{
		Server: ServerConfig{
			Port: viper.GetString("SERVER_PORT"),
			Env:  viper.GetString("SERVER_ENV"),
		},
		DB: DBConfig{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetString("DB_PORT"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			Name:     viper.GetString("DB_NAME"),
			SSLMode:  viper.GetString("DB_SSLMODE"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("REDIS_HOST"),
			Port:     viper.GetString("REDIS_PORT"),
			Password: viper.GetString("REDIS_PASSWORD"),
		},
		JWT: JWTConfig{
			Secret:     viper.GetString("JWT_SECRET"),
			AccessTTL:  viper.GetString("JWT_ACCESS_TTL"),
			RefreshTTL: viper.GetString("JWT_REFRESH_TTL"),
		},
		Twilio: TwilioConfig{
			AccountSID: viper.GetString("TWILIO_ACCOUNT_SID"),
			AuthToken:  viper.GetString("TWILIO_AUTH_TOKEN"),
			VerifySID:  viper.GetString("TWILIO_VERIFY_SID"),
		},
	}

	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.Server.Env == "" {
		cfg.Server.Env = "development"
	}
	if cfg.DB.Host == "" {
		return nil, fmt.Errorf("DB_HOST не задан")
	}

	return cfg, nil
}

func (d *DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Europe/Warsaw",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}
