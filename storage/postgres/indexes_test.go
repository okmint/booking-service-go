package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDatabaseIndexesAndQueries(t *testing.T) {
	ctx := context.Background()

	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("не удалось запустить контейнер postgres: %v", err)
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("ошибка при остановке контейнера: %v", err)
		}
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("не удалось получить строку подключения: %v", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("не удалось подключиться к БД для миграций: %v", err)
	}
	defer db.Close()

	migrationsDir := "../../migrations"
	if err := goose.Up(db, migrationsDir); err != nil {
		t.Fatalf("не удалось применить миграции: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("не удалось создать pgxpool: %v", err)
	}
	defer pool.Close()

	expectedIndexes := []string{
		"idx_bookings_stuck_cancellations",
		"idx_bookings_created_at",
		"idx_bookings_user_id",
		"idx_bookings_resource_id",
		"idx_bookings_status",
		"idx_bookings_id_desc",
		"idx_booking_history_booking_id_created_at",
		"idx_outbox_messages_status_created_at",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		query := `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`
		err := pool.QueryRow(ctx, query, idx).Scan(&exists)
		if err != nil {
			t.Errorf("ошибка при проверке индекса %s: %v", idx, err)
		}
		if !exists {
			t.Errorf("индекс %s отсутствует в базе данных", idx)
		}
	}

	repo := NewBookingsRepository(pool)

	// Тест зависших отмен
	_, err = repo.GetStuckCancellations(ctx, time.Now(), 10)
	if err != nil {
		t.Errorf("GetStuckCancellations упал с ошибкой: %v", err)
	}

	// Тест статистики
	_, err = repo.GetStatistics(ctx, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Errorf("GetStatistics упал с ошибкой: %v", err)
	}
}
