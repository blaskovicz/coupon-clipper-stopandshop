package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/sirupsen/logrus"
)

func getTicker() (<-chan time.Time, error) {
	numSeconds := 60 // one minute ticks
	if numSecondsRaw := os.Getenv("TICK_INTERVAL_SECONDS"); numSecondsRaw != "" {
		var err error
		numSeconds, err = strconv.Atoi(numSecondsRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid tick interval: %s", err)
		} else if numSeconds < 30 {
			return nil, fmt.Errorf("invalid tick interval: must be at least 30 seconds")
		}
	}
	return time.Tick(time.Second * time.Duration(numSeconds)), nil
}
func main() {
	ticker, err := getTicker()
	if err != nil {
		panic(err)
	}
	for range ticker {
		logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "loop-start"}).Info()
		// check for coupons
		client := stopandshop.New()
		if err = client.Login(os.Getenv("USERNAME"), os.Getenv("PASSWORD")); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "login"}).Error(err)
			continue
		}
		profile, err := client.ReadProfile()
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "read-profile"}).Error(err)
			continue
		}
		coupons, err := client.ReadCoupons(profile.CardNumber)
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "read-coupons"}).Error(err)
			continue
		}

		// TODO: email out "free" coupons
		if coupons == nil || len(coupons) == 0 {
			continue
		}
		for _, coupon := range coupons {
			couponRaw := strings.ToLower(fmt.Sprintf("%#v", coupon))
			// TODO ignore: buy N get one, free as part of another word, etc
			if strings.Contains(couponRaw, "free") {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "found-coupon", "coupon": couponRaw}).Info()
			}
		}
	}
}
