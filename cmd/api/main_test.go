package main

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSkipTracePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{"/metrics", true},
		{"/health", true},
		{"/v1/health", true},
		{"/ready", true},
		{"/ready/something", true},
		{"/v1/ready", true},
		{"/v1/ready/something", true},
		{"/v1/memories", false},
		{"/api/stats", false},
		{"/", false},
	}

	for _, tc := range cases {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		var got bool
		app.Get(tc.path, func(c *fiber.Ctx) error {
			got = skipTracePath(c)
			return c.SendStatus(200)
		})
		resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got != tc.want {
			t.Errorf("skipTracePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestMin(t *testing.T) {
	t.Parallel()

	cases := []struct{ a, b, want int }{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 5, -1},
		{5, -1, -1},
		{100, 100, 100},
	}

	for _, tc := range cases {
		if got := min(tc.a, tc.b); got != tc.want {
			t.Errorf("min(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
