package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	platformeventbus "github.com/lihongjie0209/microservice-platform-go/eventbus"
	platformoutbox "github.com/lihongjie0209/microservice-platform-go/outbox"
	"github.com/lihongjie0209/rule-service/internal/config"
	"go.uber.org/fx"
)

type ruleEventRuntime struct {
	config config.Config
	store  *platformoutbox.SQLStore
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
	bus    *platformeventbus.Bus
}

func newRuleOutboxStore(db *sqlx.DB) (*platformoutbox.SQLStore, error) {
	if db == nil {
		return nil, nil
	}
	return platformoutbox.NewSQLStore(db, "rule_outbox_events")
}

func newRuleEventRuntime(lifecycle fx.Lifecycle, cfg config.Config, store *platformoutbox.SQLStore, logger *slog.Logger) *ruleEventRuntime {
	runtime := &ruleEventRuntime{config: cfg, store: store, logger: logger}
	lifecycle.Append(fx.Hook{OnStart: runtime.start, OnStop: runtime.stop})
	return runtime
}

func (r *ruleEventRuntime) start(ctx context.Context) error {
	if !r.config.EventBus.Enabled {
		return nil
	}
	if r.store == nil {
		return errors.New("enabled event bus requires database outbox")
	}
	bus, err := platformeventbus.New(ctx, platformeventbus.Config{URLs: r.config.EventBus.URLs, ClientName: r.config.App.Name, StreamName: "PLATFORM_EVENTS", Subjects: []string{"platform.>"}, Storage: r.config.EventBus.Storage, MaxAge: r.config.EventBus.MaxAge, DuplicateWindow: r.config.EventBus.DuplicateWindow, ConnectTimeout: r.config.EventBus.ConnectTimeout, PublishTimeout: r.config.EventBus.PublishTimeout})
	if err != nil {
		return err
	}
	dispatcher, err := platformoutbox.New(r.store, bus, platformoutbox.Config{BatchSize: r.config.EventBus.DispatchBatchSize, Lease: r.config.EventBus.DispatchLease, RetryDelay: r.config.EventBus.DispatchRetryDelay})
	if err != nil {
		_ = bus.Close()
		return err
	}
	r.bus = bus
	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Go(func() {
		ticker := time.NewTicker(r.config.EventBus.DispatchInterval)
		defer ticker.Stop()
		for {
			if _, runErr := dispatcher.RunOnce(runCtx); runErr != nil && !errors.Is(runErr, context.Canceled) {
				r.logger.ErrorContext(runCtx, "dispatch rule outbox failed", "error", runErr)
			}
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	return nil
}

func (r *ruleEventRuntime) stop(context.Context) error {
	if r.cancel != nil {
		r.cancel()
		r.wg.Wait()
	}
	if r.bus != nil {
		return r.bus.Close()
	}
	return nil
}

var EventBusModule = fx.Module("rule-event-bus", fx.Provide(newRuleOutboxStore, newRuleEventRuntime), fx.Invoke(func(*ruleEventRuntime) {}))
