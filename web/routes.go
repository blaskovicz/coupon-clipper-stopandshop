package web

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"sort"

	"github.com/blaskovicz/coupon-clipper-stopandshop/common"
	stopandshop "github.com/blaskovicz/go-stopandshop"
	"github.com/blaskovicz/go-stopandshop/models"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

func init() {
	// for session storage
	gob.Register(&models.Token{})
}

type ByLoadStatus []models.Coupon

func (a ByLoadStatus) Len() int           { return len(a) }
func (a ByLoadStatus) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByLoadStatus) Less(i, j int) bool { return a[i].Loaded && !a[j].Loaded } // TODO if they are the same, compare by name, date, price, etc

func RouteHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("SUCCESS"))
}

func RouteClipCoupon(ss sessions.Store, cfg *common.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := getSessionOrRedirect(ss, r, w)
		if session == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "redirecting"}).Info()
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "start"}).Info()

		delete(session.Values, "flashError")
		delete(session.Values, "flashSuccess")

		if session.Values["token"] == nil {
			session.Values["flashError"] = fmt.Sprintf("Invalid auth token. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		token, _ := session.Values["token"].(*models.Token)
		client := stopandshop.New().SetToken(token)
		profile, err := client.ReadProfile()
		if err != nil {
			session.Values["flashError"] = fmt.Sprintf("Failed to read profile. Try re-logging in (%s)", err)
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
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
		logrus.WithFields(logrus.Fields{"ref": "routes.clip-coupon", "at": "coupon-clipped", "username": profile.Login, "coupon": couponID}).Info()
		session.Values["flashSuccess"] = fmt.Sprintf("Clipped coupon %s successfully!", couponID)
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
}

func RouteIndex(ss sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := getSessionOrRedirect(ss, r, w)
		if session == nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "redirecting"}).Info()
			return
		}
		logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "start"}).Info()

		if session.Values["token"] == nil {
			session.Values["flashError"] = fmt.Sprintf("Invalid auth token. Try re-logging in.")
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}

		templateData := map[string]interface{}{
			"flashError":   session.Values["flashError"],
			"flashSuccess": session.Values["flashSuccess"],
		}
		token, _ := session.Values["token"].(*models.Token)
		client := stopandshop.New().SetToken(token)
		profile, err := client.ReadProfile()
		if err != nil {
			session.Values["flashError"] = fmt.Sprintf("Failed to read profile. Try re-logging in (%s)", err)
			session.Save(r, w)
			http.Redirect(w, r, "/auth/logout", http.StatusFound)
			return
		}
		templateData["profile"] = profile

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
		if err := common.Templates.ExecuteTemplate(w, "index.tmpl", templateData); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "execute-template"}).Error(err)
		}
	}
}

func RouteLoginForm(ss sessions.Store) http.HandlerFunc {
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
		if err := common.Templates.ExecuteTemplate(w, "login.tmpl", templateData); err != nil {
			logrus.WithFields(logrus.Fields{"ref": "routes.index", "at": "execute-template"}).Error(err)
		}
	}
}

func RouteLogin(ss sessions.Store) http.HandlerFunc {
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

		delete(session.Values, "username")
		session.Values["loggedIn"] = "true"
		session.Values["flashSuccess"] = "Success. Your are now logged in."
		session.Values["token"] = client.GetToken()
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
		logrus.WithFields(logrus.Fields{"ref": "routes.login", "at": "stopandshop.login", "username": username}).Info()
	}
}

func RouteLogout(ss sessions.Store) http.HandlerFunc {
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
		if err := common.Templates.ExecuteTemplate(w, "logout.tmpl", templateData); err != nil {
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
