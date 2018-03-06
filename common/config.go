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
	EmailAllCoupons     bool   `env:"EMAIL_ALL_COUPONS"`
	SendgridAPIKey      string `env:"SENDGRID_API_KEY" required:"true"`
	LogLevel            string `env:"LOG_LEVEL" default:"info"`
	DatabaseURL         string `env:"DATABASE_URL" required:"true"`
	TickIntervalSeconds int    `env:"TICK_INTERVAL_SECONDS" default:"360"`
	Port                int    `env:"PORT" default:"3000"`
	SessionSecret       string `env:"SESSION_SECRET" required:"true"`
	EncryptionKey       string `env:"CRYPT_KEEPER_KEY" required:"true"`
}

func LoadConfig() (*Config, error) {
	var c Config
	if err := configor.Load(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
