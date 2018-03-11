package common

import (
	swarmed "github.com/blaskovicz/go-swarmed"
	"github.com/jinzhu/configor"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
	err := swarmed.LoadSecrets()
	if err != nil {
		panic(err)
	}
}

type Config struct {
	Environment     string `env:"ENVIRONMENT" default:"production"`
	EmailAllCoupons bool   `env:"EMAIL_ALL_COUPONS"`
	Email           struct {
		Username   string `env:"EMAIL_USERNAME" required:"true"`
		Password   string `env:"EMAIL_PASSWORD" required:"true"`
		ServerAddr string `env:"EMAIL_SERVER_ADDR" required:"true"` // host:port
		From       string `env:"EMAIL_FROM" required:"true"`
	}
	LogLevel            string `env:"LOG_LEVEL" default:"info"`
	DatabaseURL         string `env:"DATABASE_URL" required:"true"`
	TickIntervalSeconds int    `env:"TICK_INTERVAL_SECONDS" default:"360"`
	Port                int    `env:"PORT" default:"3000"`
	SessionSecret       string `env:"SESSION_SECRET" required:"true"`
	EncryptionKey       string `env:"CRYPT_KEEPER_KEY" required:"true"`
	AppDomain           string `env:"APP_DOMAIN" required:"true"`
}

func LoadConfig() (*Config, error) {
	var c Config
	if err := configor.Load(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
