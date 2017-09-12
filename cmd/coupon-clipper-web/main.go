package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

func RouteHealthcheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Write([]byte("SUCCESS"))
}

func main() {
	godotenv.Load()
	router := httprouter.New()
	router.GET("/healthcheck", RouteHealthcheck)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), router))
}
