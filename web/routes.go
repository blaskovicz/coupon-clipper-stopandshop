package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	"github.com/blaskovicz/go-cryptkeeper"
	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

type ByLoadStatus []models.Coupon

func (a ByLoadStatus) Len() int           { return len(a) }
func (a ByLoadStatus) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByLoadStatus) Less(i, j int) bool { return a[i].Loaded && !a[j].Loaded } // TODO if they are the same, compare by name, date, price, etc

func RouteHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("SUCCESS"))
}

func RouteClipCoupon(ss sessions.Store, cfg *common.Config, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := getSessionOrRedirect(ss, r, w)
		if session == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "redirecting"}).Info()
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "start"}).Info()

		delete(session.Values, "flashError")
		delete(session.Values, "flashSuccess")

		if session.Values["profileID"] == nil {
			session.Values["flashError"] = fmt.Sprintf("Invalid auth token. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		profileID, _ := session.Values["profileID"].(string)
		var at cryptkeeper.CryptString
		var rt cryptkeeper.CryptString
		if err := db.QueryRow("SELECT access_token,refresh_token FROM users WHERE profile_id = $1", profileID).Scan(&at, &rt); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "db.query-row"}).Warn(err)
			session.Values["flashError"] = fmt.Sprintf("Failed to read user row. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		var token models.Token
		token.AccessToken = at.String
		token.RefreshToken = &rt.String

		client := stopandshop.New().SetToken(&token)
		profile, err := client.ReadProfile()
		if err != nil {
			if stopandshop.IsAccessTokenExpired(err) {
				err = client.RefreshAccessToken()
				if err == nil {
					if _, err = db.Exec("UPDATE users SET access_token=$1, refresh_token=$2 WHERE profile_id=$3", &at, &rt, profileID); err != nil {
						logrus.Error(err)
					}
					profile, err = client.ReadProfile()
				}
			}
			if err != nil {
				logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "read-profile"}).Warn(err)
				session.Values["flashError"] = fmt.Sprintf("Failed to read profile. Try re-logging in (%s)", err)
				session.Save(r, w)
				http.Redirect(w, r, "/auth/logout", http.StatusFound)
				return
			}
		}

		couponID := mux.Vars(r)["id"]
		if couponID == "" {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "empty-coupon-id"}).Warn()
			session.Values["flashError"] = fmt.Sprintf("Invalid coupon.")
			session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		if err := client.LoadCoupon(profile.CardNumber, couponID); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "load-coupon"}).Error(err)
			session.Values["flashError"] = fmt.Sprintf("Error clipping coupon %s: %s", couponID, err)
			session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "coupon-clipped", "profile": profile.ID, "username": profile.Login, "coupon": couponID}).Info()
		session.Values["flashSuccess"] = fmt.Sprintf("Clipped coupon %s successfully!", couponID)
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
}

type profilePayload struct {
	NewCouponEmails *bool `json:"new_coupon_emails"`
	AutoClip        *bool `json:"auto_clip"`
}

func RouteUpdateProfile(cfg *common.Config, ss sessions.Store, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := ss.Get(r, "session")
		w.Header().Set("Content-Type", "application/json")
		if session == nil || session.Values["loggedIn"] == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.update-profile", "at": "not-authorized"}).Info()
			w.WriteHeader(http.StatusUnauthorized)
			resp, _ := json.Marshal(map[string]string{"id": "not_authorized", "message": "not authorized to access this resource."})
			w.Write(resp)
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.update-profile", "at": "start"}).Info()
		var pp profilePayload
		if err := json.NewDecoder(r.Body).Decode(&pp); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			resp, _ := json.Marshal(map[string]string{"id": "bad_request", "message": "invalid payload sent to resource."})
			w.Write(resp)
			return
		}
		if pp.NewCouponEmails != nil {
			if _, err := db.Exec("UPDATE users SET preferences=jsonb_set(preferences, '{new_coupon_emails}', $1) WHERE profile_id=$2", *pp.NewCouponEmails, session.Values["profileID"].(string)); err != nil {
				logrus.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				resp, _ := json.Marshal(map[string]string{"id": "internal_error", "message": "An error occurred setting new_coupon_emails."})
				w.Write(resp)
				return
			}
		}
		if pp.AutoClip != nil {
			if _, err := db.Exec("UPDATE users SET preferences=jsonb_set(preferences, '{auto_clip}', $1) WHERE profile_id=$2", *pp.AutoClip, session.Values["profileID"].(string)); err != nil {
				logrus.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				resp, _ := json.Marshal(map[string]string{"id": "internal_error", "message": "An error occurred setting auto_clip."})
				w.Write(resp)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func RouteIndex(cfg *common.Config, ss sessions.Store, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := getSessionOrRedirect(ss, r, w)
		if session == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "redirecting"}).Info()
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "start"}).Info()

		if session.Values["profileID"] == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "redirecting.session"}).Info()
			session.Values["flashError"] = fmt.Sprintf("Invalid auth token. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		profileID, _ := session.Values["profileID"].(string)
		var at cryptkeeper.CryptString
		var rt cryptkeeper.CryptString
		pB := []byte{}
		if err := db.QueryRow("SELECT access_token, refresh_token, preferences FROM users WHERE profile_id = $1", profileID).Scan(&at, &rt, &pB); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "db.query-row"}).Warn(err)
			session.Values["flashError"] = fmt.Sprintf("Failed to read user row. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		prefs := map[string]interface{}{}
		if err := json.Unmarshal(pB, &prefs); err != nil {
			logrus.Error(err)
		}
		if _, ok := prefs["new_coupon_emails"]; !ok {
			prefs["new_coupon_emails"] = true
		}
		if _, ok := prefs["auto_clip"]; !ok {
			prefs["auto_clip"] = true
		}

		var token models.Token
		token.AccessToken = at.String
		token.RefreshToken = &rt.String

		client := stopandshop.New().SetToken(&token)
		profile, err := client.ReadProfile()
		if err != nil {
			if stopandshop.IsAccessTokenExpired(err) {
				err = client.RefreshAccessToken()
				if err == nil {
					if _, err = db.Exec("UPDATE users SET access_token=$1, refresh_token=$2 WHERE profile_id=$3", &at, &rt, profileID); err != nil {
						logrus.Error(err)
					}
					profile, err = client.ReadProfile()
				}
			}
			if err != nil {
				logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "redirecting.session"}).Info()
				session.Values["flashError"] = fmt.Sprintf("Failed to read profile. Try re-logging in (%s)", err)
				session.Save(r, w)
				http.Redirect(w, r, "/auth/logout", http.StatusFound)
				return
			}
		}
		templateData := map[string]interface{}{
			"flashError":   session.Values["flashError"],
			"flashSuccess": session.Values["flashSuccess"],
			"prefs":        prefs,
			"profile":      profile,
		}

		coupons, err := client.ReadCoupons(profile.CardNumber)
		if err != nil {
			templateData["flashError"] = fmt.Sprintf("Failed to read coupons: %s", err)
		} else {
			sort.Sort(ByLoadStatus(coupons))
			templateData["coupons"] = coupons
		}

		delete(session.Values, "flashError")
		delete(session.Values, "flashSuccess")

		session.Save(r, w)
		if err := common.Templates(cfg).ExecuteTemplate(w, "index.tmpl", templateData); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "execute-template"}).Error(err)
		}
	}
}

func RouteLoginForm(cfg *common.Config, ss sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := ss.Get(r, "session")
		if session.Values["loggedIn"] != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		templateData := map[string]interface{}{
			"loggedIn":     session.Values["loggedIn"],
			"flashError":   session.Values["flashError"],
			"flashSuccess": session.Values["flashSuccess"],
			"username":     session.Values["username"],
			"usernameE":    session.Values["usernameE"],
			"passwordE":    session.Values["passwordE"],
		}
		delete(session.Values, "flashError")
		delete(session.Values, "flashSuccess")
		delete(session.Values, "username")
		delete(session.Values, "usernameE")
		delete(session.Values, "passwordE")
		session.Save(r, w)
		if err := common.Templates(cfg).ExecuteTemplate(w, "login.tmpl", templateData); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "execute-template"}).Error(err)
		}
	}
}

func RouteLogin(cfg *common.Config, ss sessions.Store, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// if they are logged in, go back to index
		session, _ := ss.Get(r, "session")
		if session.Values["loggedIn"] != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		delete(session.Values, "flashError")
		delete(session.Values, "flashSuccess")
		delete(session.Values, "username")
		delete(session.Values, "usernameE")
		delete(session.Values, "passwordE")

		// else not logged in, so try to log in
		client := stopandshop.New()
		username := r.FormValue("username")
		password := r.FormValue("password")

		var hadError bool
		if username == "" {
			session.Values["usernameE"] = "Username cannot be empty."
			hadError = true
		} else {
			session.Values["username"] = username
		}
		if password == "" {
			session.Values["passwordE"] = "Password cannot be empty."
			hadError = true
		}
		if hadError {
			logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "form-validation"}).Warn()
			session.Save(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		if err := client.Login(username, password); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "stopandshop.login"}).Warn()
			session.Values["flashError"] = "Login failed. Please check your username and password."
			session.Save(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		profile, err := client.ReadProfile()
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "stopandshop.read-profile"}).Warn(err)
			session.Values["flashError"] = "Login failed. Couldn't fetch profile."
			session.Save(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		at := cryptkeeper.CryptString{client.Token().AccessToken}
		rt := cryptkeeper.CryptString{*client.Token().RefreshToken}
		_, err = db.Exec("INSERT INTO users(profile_id, access_token, refresh_token, preferences, internal_state, last_login) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (profile_id) DO UPDATE SET access_token=$2, refresh_token=$3, last_login=$6",
			profile.ID, &at, &rt, []byte("{}"), []byte("{}"), time.Now())
		if err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "db.upsert"}).Warn(err)
			session.Values["flashError"] = "Login failed. Couldn't create or update profile."
			session.Save(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		delete(session.Values, "username")
		session.Values["loggedIn"] = "true"
		session.Values["flashSuccess"] = "Success. You are now logged in."
		session.Values["profileID"] = profile.ID
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
		logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "stopandshop.login", "username": username, "profile": profile.ID}).Info()
	}
}

func RouteLogout(cfg *common.Config, ss sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// clear existing session
		session, _ := ss.Get(r, "session")
		session.Values["flashSuccess"] = "You have been logged out."
		templateData := map[string]interface{}{
			"flashError":   session.Values["flashError"],
			"flashSuccess": session.Values["flashSuccess"],
		}
		session.Options.MaxAge = -1
		session.Save(r, w)
		logrus.WithFields(logrus.Fields{"ref": "routes.logout", "at": "finish"}).Info()
		if err := common.Templates(cfg).ExecuteTemplate(w, "logout.tmpl", templateData); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "execute-template"}).Error(err)
		}
	}
}

// TODO move to middleware
func getSessionOrRedirect(ss sessions.Store, r *http.Request, w http.ResponseWriter) *sessions.Session {
	session, _ := ss.Get(r, "session")
	if session.Values["loggedIn"] == nil {
		// TODO: handle ?next=/otherroute
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return nil
	}
	return session
}
