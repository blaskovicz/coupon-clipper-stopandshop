package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/joho/godotenv"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/sirupsen/logrus"
)

func getTicker() (<-chan time.Time, error) {
	numSeconds := 60 // one minute ticks
	if numSecondsRaw := os.Getenv("TICK_INTERVAL_SECONDS"); numSecondsRaw != "" {
		var err error
		numSeconds, err = strconv.Atoi(numSecondsRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid tick interval: %s", err)
		} else if numSeconds < 5 {
			return nil, fmt.Errorf("invalid tick interval: must be at least 5 seconds")
		}
	}
	return time.Tick(time.Second * time.Duration(numSeconds)), nil
}

var freebieRe = regexp.MustCompile("\\bfree\\b")

func main() {
	godotenv.Load()
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

		if coupons == nil || len(coupons) == 0 {
			continue
		}
		for _, coupon := range coupons {
			coupon.LegalText = "" // I don't care about searching this field
			couponRaw := strings.ToLower(fmt.Sprintf("%#v", coupon))
			// make sure it's not part of another word
			if freebieRe.FindString(couponRaw) == "" {
				continue
			} else if strings.Contains(couponRaw, "buy") && strings.Contains(couponRaw, "get one") {
				// ignore buy N get one
				continue
			} else if strings.Contains(strings.ToLower(coupon.Title), "save") {
				// eg "save $2.00"
				continue
			}

			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "found-coupon", "coupon": couponRaw}).Info()
			if err = emailCoupon(*profile, coupon); err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon", "coupon": couponRaw, "to": profile.Login}).Error(err)
				continue
			}
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon-complete", "coupon": couponRaw, "to": profile.Login}).Info()
		}
	}
}

func emailCoupon(profile models.Profile, coupon models.Coupon) error {
	from := mail.NewEmail("Coupon Clipper StopAndShop", "noreply@coupon-clipper-stopandshop.herokuapp.com")
	subject := fmt.Sprintf("[NEW] %s", coupon.Title)
	to := mail.NewEmail(profile.FirstName, profile.Login)
	content := mail.NewContent("text/html", fmt.Sprintf(`
	<html>
		<body>
			<h3>%s</h3>
			<div style='border: 1px solid #000; width: 600px; height: 200px; padding: 10px'>
				<div style='display:inline-block;width:150px'>
					<img src='%s' alt='coupon image' style='display: inline-block'/>
				</div>
				<div style='display:inline-block; overflow-y: auto; width:450px'>
					<p style='font-weight:bold'>%s <small>[Valid %s to %s]</small></p>
					<p style='color:gray'>%s</p>
				</div>
			</div>
		</body>
	</html>
`, coupon.Name, coupon.URL, coupon.Title, coupon.StartDate, coupon.EndDate, coupon.Description))
	m := mail.NewV3MailInit(from, subject, to, content)
	// TODO add link to load to card

	request := sendgrid.GetRequest(os.Getenv("SENDGRID_API_KEY"), "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(m)
	_, err := sendgrid.API(request)
	if err != nil {
		return fmt.Errorf("Failed to send coupon: %s", err)
	}
	return nil
}
