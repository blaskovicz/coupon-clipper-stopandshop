package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	"github.com/blaskovicz/coupon-clipper-stopandshop/web"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

func setupLogger(cfg *common.Config) {
	ll := strings.ToLower(cfg.LogLevel)
	var level logrus.Level
	switch ll {
	case "debug", "verbose":
		level = logrus.DebugLevel
	case "info":
		level = logrus.InfoLevel
	case "warning", "warn":
		level = logrus.WarnLevel
	case "error":
		level = logrus.ErrorLevel
	case "fatal":
		level = logrus.FatalLevel
	case "panic":
		level = logrus.PanicLevel
	default:
		panic(fmt.Sprintf("Unknown log level %s", ll))
	}
	logrus.SetLevel(level)
}
func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		logrus.Fatal(err)
	}
	setupLogger(cfg)
	db, err := common.ConnectDB(cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	var store = sessions.NewCookieStore([]byte(cfg.SessionSecret))

	r := mux.NewRouter()
	r.HandleFunc("/", web.RouteIndex(cfg, store, db)).Methods("GET")
	r.HandleFunc("/auth/login", web.RouteLoginForm(store)).Methods("GET")
	r.HandleFunc("/auth/login", web.RouteLogin(cfg, store, db)).Methods("POST")
	r.HandleFunc("/auth/logout", web.RouteLogout(store)).Methods("GET")
	r.HandleFunc("/coupons/{id}/clip", web.RouteClipCoupon(store, cfg, db)).Methods("GET")
	r.HandleFunc("/healthcheck", web.RouteHealthcheck).Methods("GET")

	logrus.Infof("Server staring on port %d", cfg.Port)
	logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), r))
}
