package main

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/go-redis/redis"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/sirupsen/logrus"
)

func getRedis(cfg *common.Config) (*redis.Client, error) {
	u, err := url.Parse(cfg.RedisURL)
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
	})
	if _, err = c.Ping().Result(); err != nil {
		return nil, err
	}
	return c, nil
}

func getTicker(cfg *common.Config) (<-chan time.Time, error) {
	if cfg.TickIntervalSeconds < 5 {
		return nil, fmt.Errorf("invalid tick interval: must be at least 5 seconds")
	}
	return time.Tick(time.Second * time.Duration(cfg.TickIntervalSeconds)), nil
}

var freebieRe = regexp.MustCompile("\\bfree\\b")

func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		logrus.Fatal(err)
	}
	ticker, err := getTicker(cfg)
	if err != nil {
		logrus.Fatal(err)
	}
	for range ticker {
		logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "loop-start"}).Info()
		// TODO connection pool
		rClient, err := getRedis(cfg)
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "connect-redis"}).Error(err)
			continue
		}
		// check for coupons
		client := stopandshop.New()
		if err = client.Login(cfg.Username, cfg.Password); err != nil {
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
			// look for "free" as its own word without any "buy one get one free" phrases...
			if fieldValue := strings.ToLower(coupon.Title); !strings.Contains(fieldValue, "free") || strings.Contains(fieldValue, "buy") || freebieRe.FindString(fieldValue) == "" {
				continue
			} else if fieldValue := strings.ToLower(coupon.Description); strings.Contains(fieldValue, "buy") {
				continue
			}

			key := fmt.Sprintf("sent_coupons:%s", profile.ID)
			sentCoupon, err := rClient.SIsMember(key, coupon.ID).Result()
			couponString := fmt.Sprintf("%#v", coupon)
			if err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "redis.sismember", "coupon": couponString}).Error(err)
				continue
			} else if sentCoupon && !cfg.EmailAllCoupons {
				continue // don't re-send coupon
			}

			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "found-coupon", "coupon": couponString}).Info()
			if err = emailCoupon(cfg, *profile, coupon); err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon", "coupon": couponString, "to": profile.Login}).Error(err)
				continue
			}
			if err := rClient.SAdd(key, coupon.ID).Err(); err != nil {
				// failed to persist the coupon in our sent items list, will re-email next run
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "redis.sadd", "coupon": couponString, "to": profile.Login}).Error(err)
				continue
			}
			// TODO reap old coupon records that have passed coupon.expirationDate
			logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon-complete", "coupon": couponString, "to": profile.Login}).Info()
		}
	}
}

var t = template.Must(template.New("email").Parse(`
	<html>
		<body>
			<div style='border: 1px solid #000; width: 440px; min-height: 100px; padding: 5px'>
				<h3>{{.Name}}: {{.Title}} from {{.StartDate}} to {{.EndDate}}</h3>
				<img src='{{.URL}}' alt='coupon image' style='display: inline-block; width: 50px; height: 50px'/>
				<div style='display:inline-block; overflow: auto; width:350px'>
					<p style='color:gray'>{{.Description}}</p>
				</div>
				<div style='margin-top: 5px'>
					<a target='_blank' href='https://coupon-clipper-stopandshop.herokuapp.com/coupons/{{.ID}}/clip'>Clip</a>
				</div>
			</div>
		</body>
	</html>
`))

func emailCoupon(cfg *common.Config, profile models.Profile, coupon models.Coupon) error {
	from := mail.NewEmail("Coupon Clipper StopAndShop", "noreply@coupon-clipper-stopandshop.herokuapp.com")
	subject := fmt.Sprintf("[NEW] %s: %s", html.EscapeString(coupon.Name), html.EscapeString(coupon.Title))
	to := mail.NewEmail(profile.FirstName, profile.Login)
	var buff bytes.Buffer
	if err := t.Execute(&buff, coupon); err != nil {
		return fmt.Errorf("Failed to generate email: %s", err)
	}
	content := mail.NewContent("text/html", buff.String())

	m := mail.NewV3MailInit(from, subject, to, content)
	// TODO add link to load to card

	request := sendgrid.GetRequest(cfg.SendgridAPIKey, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(m)
	if _, err := sendgrid.API(request); err != nil {
		return fmt.Errorf("Failed to send coupon: %s", err)
	}
	return nil
}
