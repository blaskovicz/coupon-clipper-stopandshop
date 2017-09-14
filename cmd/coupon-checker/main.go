package main

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/go-redis/redis"
	"github.com/joho/godotenv"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/sirupsen/logrus"
)

func getRedis() (*redis.Client, error) {
	u, err := url.Parse(os.Getenv("REDIS_URL"))
	if err != nil {
		return nil, err
	}
	var password string
	if u.User != nil {
		password, _ = u.User.Password()
	}
	c := redis.NewClient(&redis.Options{
		Addr:     u.Host,
		Password: password,
		DB:       3,
	})
	if _, err = c.Ping().Result(); err != nil {
		return nil, err
	}
	return c, nil
}

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
		// TODO connection pool
		rClient, err := getRedis()
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "connect-redis"}).Error(err)
			continue
		}
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
			if os.Getenv("EMAIL_ALL_COUPONS") != "true" {
				if freebieRe.FindString(couponRaw) == "" {
					// make sure it's not part of another word
					continue
				} else if strings.Contains(couponRaw, "buy") && strings.Contains(couponRaw, "get one") {
					// ignore buy N get one
					continue
				} else if strings.Contains(strings.ToLower(coupon.Title), "save") {
					// eg "save $2.00"
					continue
				}
			}

			key := fmt.Sprintf("sent_coupons:%s", profile.ID)
			sentCoupon, err := rClient.SIsMember(key, coupon.ID).Result()
			if err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "redis.sismember", "coupon": couponRaw}).Error(err)
				continue
			} else if sentCoupon {
				continue // don't re-send coupon
			}

			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "found-coupon", "coupon": couponRaw}).Info()
			if err = emailCoupon(*profile, coupon); err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon", "coupon": couponRaw, "to": profile.Login}).Error(err)
				continue
			}
			if err := rClient.SAdd(key, coupon.ID).Err(); err != nil {
				// failed to persist the coupon in our sent items list, will re-email next run
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "redis.sadd", "coupon": couponRaw, "to": profile.Login}).Error(err)
				continue
			}
			// TODO reap old coupon records that have passed coupon.expirationDate
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
				<div>
					<a target='_blank' href='https://coupon-clipper-stopandshop.herokuapp.com/coupons/%s/clip' style='border: 1px dotted gray'>Clip</a>
				</div>
			</div>
		</body>
	</html>
`, coupon.Name, coupon.URL, coupon.Title, coupon.StartDate, coupon.EndDate, coupon.Description, coupon.ID))
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
