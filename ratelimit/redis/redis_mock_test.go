package redis_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	goredis "github.com/redis/go-redis/v9"

	rlredis "github.com/TykTechnologies/midsommar/v2/ratelimit/redis"
)

var (
	errRedis  = errors.New("redis error")
	acceptAny redismock.CustomMatch = func(_, _ []interface{}) error { return nil }
)

func TestRecord_Success(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.MatchExpectationsInOrder(true)
	mock.CustomMatch(acceptAny).ExpectZRemRangeByScore("test:mykey", "", "").SetVal(0)
	mock.CustomMatch(acceptAny).ExpectZAdd("test:mykey", goredis.Z{Score: 0, Member: ""}).SetVal(1)
	mock.ExpectZCard("test:mykey").SetVal(5)
	mock.CustomMatch(acceptAny).ExpectExpire("test:mykey", 0).SetVal(true)

	count, err := backend.Record(ctx, "mykey", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected count 5, got %d", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRecord_PipelineError(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.MatchExpectationsInOrder(true)
	mock.CustomMatch(acceptAny).ExpectZRemRangeByScore("test:mykey", "", "").SetErr(errRedis)
	mock.CustomMatch(acceptAny).ExpectZAdd("test:mykey", goredis.Z{Score: 0, Member: ""}).SetErr(errRedis)
	mock.ExpectZCard("test:mykey").SetErr(errRedis)
	mock.CustomMatch(acceptAny).ExpectExpire("test:mykey", 0).SetErr(errRedis)

	_, err := backend.Record(ctx, "mykey", time.Minute)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCount_Success(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.CustomMatch(acceptAny).ExpectZCount("test:mykey", "", "").SetVal(3)

	count, err := backend.Count(ctx, "mykey", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestCount_Error(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.CustomMatch(acceptAny).ExpectZCount("test:mykey", "", "").SetErr(errRedis)

	_, err := backend.Count(ctx, "mykey", time.Minute)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReset_MockSuccess(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.ExpectDel("test:mykey").SetVal(1)

	if err := backend.Reset(ctx, "mykey"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReset_MockError(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.ExpectDel("test:mykey").SetErr(errRedis)

	if err := backend.Reset(ctx, "mykey"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOldest_Success(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	now := time.Now()
	nanos := float64(now.UnixNano())

	mock.CustomMatch(acceptAny).ExpectZRangeByScoreWithScores("test:mykey", &goredis.ZRangeBy{
		Min:    "0",
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}).SetVal([]goredis.Z{
		{Score: nanos, Member: "some-uuid"},
	})

	oldest, err := backend.Oldest(ctx, "mykey", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldest.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if oldest.Sub(now).Abs() > time.Second {
		t.Fatalf("oldest %v too far from now %v", oldest, now)
	}
}

func TestOldest_Empty(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.CustomMatch(acceptAny).ExpectZRangeByScoreWithScores("test:mykey", &goredis.ZRangeBy{
		Min:    "0",
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}).SetVal(nil)

	oldest, err := backend.Oldest(ctx, "mykey", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !oldest.IsZero() {
		t.Fatalf("expected zero time for empty result, got %v", oldest)
	}
}

func TestOldest_Error(t *testing.T) {
	client, mock := redismock.NewClientMock()
	defer client.Close()

	backend := rlredis.New(client, "test:")
	ctx := t.Context()

	mock.CustomMatch(acceptAny).ExpectZRangeByScoreWithScores("test:mykey", &goredis.ZRangeBy{
		Min:    "0",
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}).SetErr(errRedis)

	_, err := backend.Oldest(ctx, "mykey", time.Minute)
	if err == nil {
		t.Fatal("expected error")
	}
}
