FROM golang:1.9
WORKDIR /go/src/github.com/blaskovicz/coupon-clipper-stopandshop
COPY . .
RUN go-wrapper install ./cmd/...
EXPOSE 6091
ENV SENDGRID_API_KEY=mykey LOG_LEVEL=info DATABASE_URL=postgres://user:pass@host/db PORT=6091 SESSION_SECRET=mysecret CRYPT_KEEPER_KEY=cryptickey TICK_INTERVAL_SECONDS=360
CMD ["coupon-clipper-web"]