package health

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// CheckDB on a healthy DB returns OK with non-zero latency.
func TestCheckDB_Healthy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	res := CheckDB(context.Background(), db)
	if !res.OK {
		t.Errorf("expected OK, detail=%q", res.Detail)
	}
	if res.Name != "db" {
		t.Errorf("name should be 'db', got %q", res.Name)
	}
}

// nil DB returns a structured "down" result, not a panic.
func TestCheckDB_NilSafe(t *testing.T) {
	res := CheckDB(context.Background(), nil)
	if res.OK {
		t.Errorf("nil DB should report not OK")
	}
}

// DB error surfaces in Detail.
func TestCheckDB_QueryError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT 1").WillReturnError(context.DeadlineExceeded)

	res := CheckDB(context.Background(), db)
	if res.OK {
		t.Errorf("error should be not OK")
	}
	if res.Detail == "" {
		t.Errorf("Detail should describe the error")
	}
}

// CheckRedis on a live (miniredis) client returns OK.
func TestCheckRedis_Healthy(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli.Close()
	res := CheckRedis(context.Background(), cli)
	if !res.OK {
		t.Errorf("expected OK, got %+v", res)
	}
}

// CheckRedis on a nil client returns a structured "down" result.
func TestCheckRedis_NilSafe(t *testing.T) {
	res := CheckRedis(context.Background(), nil)
	if res.OK {
		t.Errorf("nil client should report not OK")
	}
	if res.Detail == "" {
		t.Errorf("Detail should explain")
	}
}

// CheckWorker reads the heartbeat timestamp.
func TestCheckWorker_FreshHeartbeat(t *testing.T) {
	hb := &WorkerHeartbeat{}
	hb.Touch()
	res := CheckWorker("thumb", hb, 1*time.Minute)
	if !res.OK {
		t.Errorf("fresh heartbeat should be OK, got %+v", res)
	}
}

// A heartbeat that's older than maxAge is stale → not OK.
func TestCheckWorker_StaleHeartbeat(t *testing.T) {
	hb := &WorkerHeartbeat{}
	hb.Touch()
	// Wait so the heartbeat is stale.
	time.Sleep(50 * time.Millisecond)
	res := CheckWorker("thumb", hb, 10*time.Millisecond)
	if res.OK {
		t.Errorf("stale heartbeat should not be OK")
	}
}

// Never-touched heartbeat is not OK.
func TestCheckWorker_NeverStarted(t *testing.T) {
	hb := &WorkerHeartbeat{}
	res := CheckWorker("thumb", hb, 1*time.Hour)
	if res.OK {
		t.Errorf("never-started worker should not be OK")
	}
	if res.Detail == "" {
		t.Errorf("Detail should explain")
	}
}

// nil heartbeat returns a structured down result.
func TestCheckWorker_NilSafe(t *testing.T) {
	res := CheckWorker("thumb", nil, 1*time.Minute)
	if res.OK {
		t.Errorf("nil heartbeat should not be OK")
	}
}

// Heartbeat name is prefixed with "worker:" so the /readyz JSON has a
// stable namespace and dashboards can filter.
func TestCheckWorker_NameFormat(t *testing.T) {
	hb := &WorkerHeartbeat{}
	hb.Touch()
	res := CheckWorker("thumb", hb, 1*time.Minute)
	if res.Name != "worker:thumb" {
		t.Errorf("name should be 'worker:thumb', got %q", res.Name)
	}
}

// Heartbeat updates are atomic — Touch from many goroutines must not race
// or corrupt. Run with -race to catch.
func TestWorkerHeartbeat_ConcurrentTouch(t *testing.T) {
	hb := &WorkerHeartbeat{}
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				hb.Touch()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	res := CheckWorker("x", hb, 1*time.Hour)
	if !res.OK {
		t.Errorf("expected OK after many touches, got %+v", res)
	}
}
