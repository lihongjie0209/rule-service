//go:build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformprincipal "github.com/lihongjie0209/microservice-platform-go/principal"
	"github.com/lihongjie0209/rule-service/internal/config"
	appdb "github.com/lihongjie0209/rule-service/internal/database"
	"github.com/lihongjie0209/rule-service/internal/migration"
	"github.com/lihongjie0209/rule-service/internal/rule"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestRepositoryAndMigrations(t *testing.T) {
	for _, databaseType := range []string{"postgres", "mysql"} {
		t.Run(databaseType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			dsn, migrationURL := startDatabase(t, ctx, databaseType)
			migrationPath, err := filepath.Abs(filepath.Join("..", "migrations", databaseType))
			if err != nil {
				t.Fatal(err)
			}
			schema := ""
			if databaseType == "postgres" {
				schema = "integration_postgres"
			}
			migrationCfg := config.Migration{Path: migrationPath, DatabaseURL: migrationURL, Table: "integration_" + databaseType + "_schema_migrations", Schema: schema, CreateSchema: schema != ""}
			migrationErrors := make(chan error, 3)
			var migrations sync.WaitGroup
			for range 3 {
				migrations.Add(1)
				go func() {
					defer migrations.Done()
					migrationErrors <- migration.Run(migrationCfg, "up", 0)
				}()
			}
			migrations.Wait()
			close(migrationErrors)
			for err := range migrationErrors {
				if err != nil {
					t.Fatalf("concurrent migration up: %v", err)
				}
			}

			db, err := appdb.Open(ctx, config.Database{Type: databaseType, DSN: dsn, Schema: schema, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute, PingTimeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			var userTables int
			if databaseType == "postgres" {
				if err := db.GetContext(ctx, &userTables, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = 'users'`); err != nil {
					t.Fatal(err)
				}
				var timezone string
				if err := db.GetContext(ctx, &timezone, `SHOW TIMEZONE`); err != nil || timezone != "Asia/Shanghai" {
					t.Fatalf("timezone=%q err=%v", timezone, err)
				}
			} else if err := db.GetContext(ctx, &userTables, `SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'users'`); err != nil {
				t.Fatal(err)
			}
			if userTables != 0 {
				t.Fatal("generic template migration must not create a users table")
			}
			actorContext := platformprincipal.WithContext(ctx, platformprincipal.Principal{ID: "integration", Type: platformprincipal.TypeServiceAccount})
			service := rule.NewService(rule.NewRepository(db), appdb.NewTransactor(db))
			ruleSet, err := service.CreateRuleSet(actorContext, "tenant-integration", "checkout.discount", "Checkout discount", "")
			if err != nil {
				t.Fatalf("create rule set: %v", err)
			}
			definition := `{"rules":[{"name":"vip","condition":"facts.vip == true","result":{"discount":20}}],"default_result":{"discount":0}}`
			version, created, err := service.CreateRuleVersion(actorContext, "tenant-integration", ruleSet.ID, definition, "version-key-1")
			if err != nil || !created {
				t.Fatalf("create rule version: created=%v err=%v", created, err)
			}
			_, duplicate, err := service.CreateRuleVersion(actorContext, "tenant-integration", ruleSet.ID, definition, "version-key-1")
			if err != nil || duplicate {
				t.Fatalf("replay rule version: created=%v err=%v", duplicate, err)
			}
			ruleSet, version, err = service.PublishRuleVersion(actorContext, "tenant-integration", ruleSet.ID, version.ID, ruleSet.Version, version.Version)
			if err != nil {
				t.Fatalf("publish rule version: %v", err)
			}
			evaluation, _, err := service.Evaluate(actorContext, "tenant-integration", ruleSet.ID, "", version.VersionNumber, `{"vip":true}`)
			if err != nil || !evaluation.Matched || evaluation.MatchedRule != "vip" {
				t.Fatalf("evaluation=%+v err=%v", evaluation, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := migration.Run(migrationCfg, "down", 0); err != nil {
				t.Fatalf("migration down: %v", err)
			}
		})
	}
}

func startDatabase(t *testing.T, ctx context.Context, databaseType string) (string, string) {
	t.Helper()
	switch databaseType {
	case "postgres":
		container, err := postgres.Run(ctx, "postgres:17-alpine", postgres.WithDatabase("app"), postgres.WithUsername("app"), postgres.WithPassword("app"), postgres.BasicWaitStrategies(), postgres.WithSQLDriver("pgx"))
		if err != nil {
			t.Fatal(err)
		}
		testcontainers.CleanupContainer(t, container)
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
		return dsn, dsn
	case "mysql":
		container, err := mysql.Run(ctx, "mysql:8.4", mysql.WithDatabase("app"), mysql.WithUsername("app"), mysql.WithPassword("app"))
		if err != nil {
			t.Fatal(err)
		}
		testcontainers.CleanupContainer(t, container)
		dsn, err := container.ConnectionString(ctx, "parseTime=true")
		if err != nil {
			t.Fatal(err)
		}
		migrationDSN, err := container.ConnectionString(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return dsn, "mysql://" + migrationDSN
	default:
		t.Fatal(fmt.Errorf("unsupported database %q", databaseType))
		return "", ""
	}
}
