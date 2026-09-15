package maildelivery

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"prods/internal/inquiries"
	"prods/internal/platform"
)

type Store interface {
	ReconcileSMTPDeliveries(context.Context) error
	ClaimSMTPDeliveryRecipient(context.Context) (inquiries.DeliveryWork, bool, error)
	CompleteSMTPDeliveryRecipient(context.Context, string, int, inquiries.DeliveryStatus, string, string) error
}

type Manager struct {
	store    Store
	sender   Sender
	wake     chan struct{}
	stop     chan struct{}
	draining atomic.Bool
	stopOnce sync.Once
	wg       sync.WaitGroup
	health   *platform.RuntimeHealthTracker
}

func NewManager(store Store, sender Sender) (*Manager, error) {
	if err := store.ReconcileSMTPDeliveries(context.Background()); err != nil {
		return nil, err
	}
	manager := &Manager{
		store: store, sender: sender, wake: make(chan struct{}, 1), stop: make(chan struct{}),
		health: platform.NewRuntimeHealthTracker(nil),
	}
	manager.wg.Add(1)
	manager.health.Started()
	go manager.run()
	manager.Wake()
	return manager, nil
}

func (manager *Manager) Wake() {
	if manager == nil || manager.draining.Load() {
		return
	}
	select {
	case manager.wake <- struct{}{}:
	default:
	}
}

func (manager *Manager) BeginDrain() {
	if manager != nil {
		manager.draining.Store(true)
	}
}

func (manager *Manager) Close() {
	if manager == nil {
		return
	}
	manager.BeginDrain()
	manager.stopOnce.Do(func() { close(manager.stop) })
	manager.wg.Wait()
}

func (manager *Manager) run() {
	defer manager.wg.Done()
	defer manager.health.Stopped()
	for {
		select {
		case <-manager.stop:
			return
		case <-manager.wake:
			manager.process()
		}
	}
}

func (manager *Manager) process() {
	for !manager.draining.Load() {
		work, found, err := manager.store.ClaimSMTPDeliveryRecipient(context.Background())
		if err != nil {
			manager.health.Failed("claim")
			slog.Error("SMTP delivery worker could not claim durable work", "error", err)
			return
		}
		if !found {
			manager.health.Succeeded()
			return
		}
		result := manager.sender.Send(context.Background(), Message{To: work.To, Subject: work.Subject, Body: work.Body})
		if err := manager.store.CompleteSMTPDeliveryRecipient(context.Background(), work.AttemptID, work.RecipientIndex,
			result.Status, result.ErrorClass, result.ErrorMessage); err != nil {
			manager.health.Failed("completion")
			slog.Error("SMTP delivery worker could not persist completion", "error", err)
			return
		}
		manager.health.Succeeded()
	}
}

func (manager *Manager) RuntimeHealth() platform.RuntimeHealthSnapshot {
	if manager == nil {
		return platform.RuntimeHealthSnapshot{}
	}
	return manager.health.Snapshot()
}
