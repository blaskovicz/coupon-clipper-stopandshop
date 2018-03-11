FROM golang:1.9
WORKDIR /go/src/github.com/blaskovicz/coupon-clipper-stopandshop
COPY . .
RUN go-wrapper install ./cmd/...
EXPOSE 6091
ENV APP_DOMAIN=coupon-clipper-stopandshop.herokuapp.com SENDGRID_API_KEY=mykey LOG_LEVEL=info DATABASE_URL=postgres://user:pass@host/db PORT=6091 SESSION_SECRET=mysecret CRYPT_KEEPER_KEY=cryptickey TICK_INTERVAL_SECONDS=360 EMAIL_FROM=noreply@localhost EMAIL_PASSWORD=serverpass EMAIL_SERVER_ADDR=localhost:25 EMAIL_USERNAME=bobbeh
CMD ["coupon-clipper-web"]
