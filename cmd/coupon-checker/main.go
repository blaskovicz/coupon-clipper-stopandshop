package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	cryptkeeper "github.com/blaskovicz/go-cryptkeeper"
	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/sirupsen/logrus"
)

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

	common.MustLoadTemplates(cfg)

	ticker, err := getTicker(cfg)
	if err != nil {
		logrus.Fatal(err)
	}
	db, err := common.ConnectDB(cfg)
	if err != nil {
		logrus.Fatal(err)
	}
	for range ticker {
		logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "loop-start"}).Info()

		// for every user
		rows, err := db.Query("SELECT id, access_token, refresh_token, preferences, internal_state FROM users")
		if err != nil {
			logrus.Fatal(err)
		}
		for rows.Next() {
			var id int
			var at cryptkeeper.CryptString
			var rt cryptkeeper.CryptString
			prefs := map[string]interface{}{}
			state := map[string]interface{}{}
			prefsB := []byte{}
			stateB := []byte{}
			if err = rows.Scan(&id, &at, &rt, &prefsB, &stateB); err != nil {
				logrus.Fatal(err)
			}

			if err := json.Unmarshal(prefsB, &prefs); err != nil {
				panic(err)
			}
			if err := json.Unmarshal(stateB, &state); err != nil {
				panic(err)
			}

			autoClip := true
			sendEmails := true
			if ac, ok := prefs["auto_clip"]; ok && !ac.(bool) {
				autoClip = false
			}
			if se, ok := prefs["new_coupon_emails"]; ok && !se.(bool) {
				sendEmails = false
			}

			if !sendEmails {
				continue
			}

			// check for coupons
			client := stopandshop.New().SetToken(&models.Token{AccessToken: at.String, RefreshToken: &rt.String})
			var newToken bool
			if err := client.RefreshAccessToken(); err != nil {
				// TODO email if shopandshop.IsRefreshTokenExpired(err)
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "refresh-access-token", "token.access": at.String, "token.refresh": rt.String}).Error(err)
				continue
			}
			t := client.Token()
			if t.AccessToken != at.String {
				at.String = t.AccessToken
				newToken = true
			}
			if *t.RefreshToken != rt.String {
				rt.String = *t.RefreshToken
				newToken = true
			}

			profile, err := client.ReadProfile()
			if err != nil {
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "read-profile", "token.access": at.String, "token.refresh": rt.String}).Error(err)
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

			var sentCoupons int
			for _, coupon := range coupons {
				// look for "free" as its own word without any "buy one get one free" phrases...
				if fieldValue := strings.ToLower(coupon.Title); !strings.Contains(fieldValue, "free") || strings.Contains(fieldValue, "buy") || freebieRe.FindString(fieldValue) == "" {
					continue
				} else if fieldValue := strings.ToLower(coupon.Description); strings.Contains(fieldValue, "buy") {
					continue
				}

				var ok bool
				var sentType map[string]interface{}
				if state["sent_coupons"] == nil {
					state["sent_coupons"] = map[string]interface{}{}
				}
				sentType, ok = state["sent_coupons"].(map[string]interface{})
				if ok && sentType[coupon.ID] != nil && !cfg.EmailAllCoupons {
					continue // already sent this coupon
				}

				couponString := fmt.Sprintf("%#v", coupon)
				if autoClip {
					logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "auto-clip", "profile": profile.Login, "coupon": couponString}).Info()
					if err := client.LoadCoupon(profile.CardNumber, coupon.ID); err != nil {
						logrus.Fatal(err)
					}
				}

				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "found-coupon", "coupon": couponString}).Info()
				if err = emailCoupon(cfg, *profile, &coupon, autoClip); err != nil {
					logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon", "coupon": couponString, "to": profile.Login}).Error(err)
					continue
				}
				sentCoupons++
				sentType[coupon.ID] = coupon.EndDate // yyyy-mm-dd
				// TODO reap old coupon records that have passed coupon.expirationDate
				logrus.WithFields(logrus.Fields{"ref": "coupon-checker", "at": "email-coupon-complete", "coupon": couponString, "to": profile.Login}).Info()
			}

			if newToken {
				if _, err = db.Exec("UPDATE users SET access_token=$1, refresh_token=$2 WHERE id=$3", &at, &rt, id); err != nil {
					logrus.Fatal(err)
				}
			}

			if sentCoupons == 0 {
				continue
			}

			b, err := json.Marshal(state)
			if err != nil {
				logrus.Fatal(err)
			}

			if _, err = db.Exec("UPDATE users SET internal_state=$1 WHERE id=$2", b, id); err != nil {
				logrus.Fatal(err)
			}
		}
	}
}

type couponEmailData struct {
	Coupon   *models.Coupon
	Config   *common.Config
	AutoClip bool
}

func emailCoupon(cfg *common.Config, profile models.Profile, coupon *models.Coupon, autoClip bool) error {
	var buff bytes.Buffer
	if err := common.Templates(cfg).ExecuteTemplate(&buff, "clip-coupon.tmpl", couponEmailData{coupon, cfg, autoClip}); err != nil {
		return fmt.Errorf("Failed to generate email: %s", err)
	}

	subject := fmt.Sprintf("[Stop&Shop Coupon] %s: %s", html.EscapeString(coupon.Name), html.EscapeString(coupon.Title))
	mailAuth := smtp.CRAMMD5Auth(cfg.Email.Username, cfg.Email.Password)
	return smtp.SendMail(cfg.Email.ServerAddr, mailAuth, cfg.Email.From, []string{profile.Login}, []byte(fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\nContent-Type: text/html\r\n\r\n%s\r\n", profile.Login, cfg.Email.From, subject, buff.String())))
}
