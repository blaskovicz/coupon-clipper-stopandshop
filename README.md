# Coupon Clipper Stop and Shop
> Automatically get reminded about free coupons!

[coupon-clipper-stopandshop.carlyzach.com](https://coupon-clipper-stopandshop.carlyzach.com/).

## Developing

Make changes, and then run `go build ./...` or `docker-compose build`.

## Deploying


To deploy to heroku:

1) `heroku create my-app-name`
2) `heroku buildpacks:set heroku/go`
3) `heroku config:set USERNAME=stop_and_shop_username PASSWORD=stop_and_shop_password`
4) `heroku addons:create heroku-redis:hobby-dev`
5) `git push heroku master`
6) visit the app to get started and configure alerts!
