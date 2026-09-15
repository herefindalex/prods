package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingDrainHandler struct {
	started  chan struct{}
	canceled chan struct{}
	once     sync.Once
	draining atomic.Bool
}

func (handler *blockingDrainHandler) BeginDrain() {
	handler.draining.Store(true)
}

func (handler *blockingDrainHandler) ServeHTTP(_ http.ResponseWriter, request *http.Request) {
	handler.once.Do(func() { close(handler.started) })
	<-request.Context().Done()
	close(handler.canceled)
}

func TestServeForceClosesRequestsAfterGracefulShutdownBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := &blockingDrainHandler{started: make(chan struct{}), canceled: make(chan struct{})}
	completed := make(chan struct{})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveListener(listener, handler, completed, 50*time.Millisecond)
	}()
	requestDone := make(chan struct{})
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/block")
		if err == nil {
			_ = response.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request did not start")
	}
	close(completed)
	select {
	case <-handler.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("forced HTTP close did not cancel the request context")
	}
	select {
	case err := <-serveDone:
		if err == nil || !strings.Contains(err.Error(), "graceful shutdown exceeded") {
			t.Fatalf("serve shutdown error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after forced close")
	}
	if !handler.draining.Load() {
		t.Fatal("handler admission was not closed before shutdown")
	}
	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("client request remained blocked after forced close")
	}
}

func TestServiceStopUsesTheSameAdmissionDrain(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := &blockingDrainHandler{started: make(chan struct{}), canceled: make(chan struct{})}
	serviceStop := make(chan struct{})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveListenerWithStop(listener, handler, nil, 50*time.Millisecond, serviceStop)
	}()
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/block")
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("service request did not start")
	}
	close(serviceStop)
	select {
	case <-handler.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("service stop did not cancel request after drain budget")
	}
	select {
	case err := <-serveDone:
		if err == nil || !strings.Contains(err.Error(), "graceful shutdown exceeded") {
			t.Fatalf("service stop result=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service stop did not finish")
	}
	if !handler.draining.Load() {
		t.Fatal("service stop skipped application admission drain")
	}
}
