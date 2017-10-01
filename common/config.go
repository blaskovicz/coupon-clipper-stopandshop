package common

import (
	"github.com/jinzhu/configor"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

type Config struct {
	EmailAllCoupons     bool   `env:"EMAIL_ALL_COUPONS"`
	SendgridAPIKey      string `env:"SENDGRID_API_KEY" required:"true"`
	Username            string `env:"USERNAME" required:"true"`
	Password            string `env:"PASSWORD" required:"true"`
	LogLevel            string `env:"LOG_LEVEL" default:"info"`
	RedisURL            string `env:"REDIS_URL" required:"true"`
	TickIntervalSeconds int    `env:"TICK_INTERVAL_SECONDS" default:"360"`
	Port                int    `env:"PORT" default:"3000"`
}

func LoadConfig() (*Config, error) {
	var c Config
	if err := configor.Load(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
