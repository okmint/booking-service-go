package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// setupTestDB поднимает контейнер, накатывает миграции и возвращает готовый пул соединений.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
	)
	if err != nil {
		t.Fatalf("не удалось запустить контейнер postgres: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Errorf("ошибка при остановке контейнера: %v", err)
		}
	})

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

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func TestDatabaseIndexesAndQueries(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewBookingsRepository(pool)

	t.Run("check_pg_indexes", func(t *testing.T) {
		expectedIndexes := []string{
			"idx_bookings_cancel_command_sent_at",
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
	})

	t.Run("query_stuck_cancellations", func(t *testing.T) {
		_, err := repo.GetStuckCancellations(ctx, time.Now(), 10)
		if err != nil {
			t.Errorf("GetStuckCancellations упал с ошибкой: %v", err)
		}
	})

	t.Run("query_statistics", func(t *testing.T) {
		_, err := repo.GetStatistics(ctx, time.Now().Add(-24*time.Hour), time.Now())
		if err != nil {
			t.Errorf("GetStatistics упал с ошибкой: %v", err)
		}
	})
}
