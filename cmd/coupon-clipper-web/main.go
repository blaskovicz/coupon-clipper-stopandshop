package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

func RouteHealthcheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Write([]byte("SUCCESS"))
}
func RouteClipCoupon(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// very basic and un-dry for v0.1
	w.Header().Set("Content-Type", "text/plain")

	client := stopandshop.New()
	if err := client.Login(os.Getenv("USERNAME"), os.Getenv("PASSWORD")); err != nil {
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

	couponID := ps.ByName("id")
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
func setupLogger() {
	ll := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if ll == "" {
		ll = "info"
	}
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
	godotenv.Load()
	setupLogger()
	router := httprouter.New()
	router.GET("/coupons/:id/clip", RouteClipCoupon)
	router.GET("/healthcheck", RouteHealthcheck)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	logrus.Infof("Server staring on port %s", port)
	logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), router))
}
