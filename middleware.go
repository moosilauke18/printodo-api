package main

import (
	"net/http"
	"time"

	jwtmiddleware "github.com/auth0/go-jwt-middleware"
	"github.com/didip/tollbooth"
	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gorilla/handlers"
)

func recoveryHandler(h http.Handler) http.Handler {
	return handlers.RecoveryHandler()(h)
}

func timeoutHandler(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, 1*time.Second, "timed out")
}

func adminTimeoutHandler(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, 15*time.Second, "timed out")
}

func jwtMiddleware(h http.Handler) http.Handler {
	jwtMiddleware := jwtmiddleware.New(jwtmiddleware.Options{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			return JWT_SECRET, nil
		},
		SigningMethod: jwt.SigningMethodHS256,
	})
	return jwtMiddleware.Handler(h)
}

func rateLimitMiddleware(h http.Handler) http.Handler {
	return tollbooth.LimitHandler(tollbooth.NewLimiter(1, nil), h)
}
