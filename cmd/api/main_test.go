package main

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestMin(t *testing.T) {
	if min(2, 5) != 2 || min(9, 3) != 3 || min(4, 4) != 4 {
		t.Fatalf("min broken: %d %d %d", min(2, 5), min(9, 3), min(4, 4))
	}
}

func TestSkipTracePath(t *testing.T) {
	app := fiber.New()

	for _, tc := range []struct {
		uri  string
		want bool
	}{
		{"/metrics", true},
		{"/health", true},
		{"/v1/health", true},
		{"/ready", true},
		{"/v1/ready", true},
		{"/ready/extra", true},
		{"/v1/ready/deep", true},
		{"/v1/memories", false},
		{"/v1/events", false},
		{"/", false},
	} {
		var acq fasthttp.RequestCtx
		acq.Request.SetRequestURI(tc.uri)
		acq.Request.Header.SetMethod("GET")
		c := app.AcquireCtx(&acq)
		got := skipTracePath(c)
		app.ReleaseCtx(c)
		if got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.uri, got, tc.want)
		}
	}
}
