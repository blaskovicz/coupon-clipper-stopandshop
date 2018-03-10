package common

import (
	"html/template"
	"sync"
  "github.com/sirupsen/logrus"
)

var templates *template.Template
var templateLocker sync.Mutex

func MustLoadTemplates(cfg *Config) {
	templateLocker.Lock()
	defer templateLocker.Unlock()
	if err := loadTemplates(cfg); err != nil {
		panic(err)
	}
}

func loadTemplates(cfg *Config) error {
	t, err := template.ParseGlob("templates/*.tmpl")
	if err != nil {
		return err
	}
	templates = t
	return nil
}

func Templates(cfg *Config) *template.Template {
	templateLocker.Lock()
	defer templateLocker.Unlock()
	if cfg.Environment == "development" {
		err := loadTemplates(cfg)
		if err != nil {
			logrus.WithField("err", err).Error("Failed to load new templates")
		}
	}
	return templates
}
