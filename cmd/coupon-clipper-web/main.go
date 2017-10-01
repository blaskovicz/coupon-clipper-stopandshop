package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func RouteHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("SUCCESS"))
}
func RouteClipCoupon(cfg *common.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		couponID := mux.Vars(r)["id"]
		// very basic and un-dry for v0.1
		w.Header().Set("Content-Type", "text/plain")

		client := stopandshop.New()
		if err := client.Login(cfg.Username, cfg.Password); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "login"}).Error(err)
			w.WriteHeader(500)
			fmt.Fprintf(w, "Error clipping coupon: %s", err)
			return
		}
		profile, err := client.ReadProfile()
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "read-profile"}).Error(err)
			w.WriteHeader(500)
			fmt.Fprintf(w, "Error clipping coupon: %s", err)
			return
		}

		if couponID == "" {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "empty-coupon-id"}).Warn()
			w.WriteHeader(400)
			fmt.Fprintf(w, "Missing coupon id")
			return
		}

		if err = client.LoadCoupon(profile.CardNumber, couponID); err == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "coupon-clipped", "coupon": couponID}).Info()
			fmt.Fprintf(w, "Clipped coupon %s successfully!", couponID)
		} else {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "load-coupon"}).Error(err)
			w.WriteHeader(500)
			fmt.Fprintf(w, "Error clipping coupon: %s", err)
			return
		}
	}
}
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
	r := mux.NewRouter()
	r.HandleFunc("/coupons/{id}/clip", RouteClipCoupon(cfg)).Methods("GET")
	r.HandleFunc("/healthcheck", RouteHealthcheck).Methods("GET")
	logrus.Infof("Server staring on port %d", cfg.Port)
	logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), r))
}
