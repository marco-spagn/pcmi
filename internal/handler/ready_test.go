package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/marco-spagn/pcmi/internal/event"
)

func TestReadyRoutes_healthy(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })
	mock.ExpectPing()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mr.Close() })

	prevRedis := event.RedisClient
	t.Cleanup(func() { event.RedisClient = prevRedis })
	event.InitRedis(mr.Addr())

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterReadyRoutes(app, mock)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Status     string `json:"status"`
		DBOK       bool   `json:"database_ok"`
		RedisOK    bool   `json:"redis_ok"`
		Service    string `json:"service"`
		VersionTag string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.DBOK || !body.RedisOK || body.Status != "ready" {
		t.Fatalf("unexpected payload: %+v", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadyRoutes_databaseDown(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })
	mock.ExpectPing().WillReturnError(errors.New("db down"))

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mr.Close() })

	prevRedis := event.RedisClient
	t.Cleanup(func() { event.RedisClient = prevRedis })
	event.InitRedis(mr.Addr())

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterReadyRoutes(app, mock)

	resp, err := app.Test(httptest.NewRequest("GET", "/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status %d want 503", resp.StatusCode)
	}
	var body struct {
		Status  string `json:"status"`
		DBOK    bool   `json:"database_ok"`
		RedisOK bool   `json:"redis_ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DBOK || body.Status == "ready" {
		t.Fatalf("expected database_ok false, got %+v", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadyRoutes_redisUnavailable(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })
	mock.ExpectPing()

	prevRedis := event.RedisClient
	t.Cleanup(func() { event.RedisClient = prevRedis })
	event.RedisClient = nil

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	RegisterReadyRoutes(app, mock)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status %d want 503", resp.StatusCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
