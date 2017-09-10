package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

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
	}
}
