package github_secondary_ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func Test_SleepWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	wg := &sync.WaitGroup{}
	wg.Add(1)

	errChan := make(chan error, 1)
	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 1*time.Second)
		if err == nil {
			errChan <- fmt.Errorf("expected error, got nil")
		} else if err != context.Canceled {
			errChan <- fmt.Errorf("expected context.Canceled, got %v", err)
		} else {
			errChan <- nil
		}
		close(errChan)
		wg.Done()
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	wg.Wait()
	if err := <-errChan; err != nil {
		t.Fatal(err.Error())
	}

	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected elapsed time to be less than 100ms, got %v", elapsed)
	}
}

func Test_SleepWithContext(t *testing.T) {
	ctx := context.Background()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	errChan := make(chan error, 1)
	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 50*time.Millisecond)
		if err != nil {
			errChan <- fmt.Errorf("expected nil, got %v", err)
		} else {
			errChan <- nil
		}
		wg.Done()
	}()

	wg.Wait()
	if err := <-errChan; err != nil {
		t.Fatal(err.Error())
	}

	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected elapsed time to be less than 100ms, got %v", elapsed)
	}
}

func Test_SleepWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	errChan := make(chan error, 1)
	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 1*time.Second)
		if err == nil {
			errChan <- fmt.Errorf("expected error, got nil")
		} else if err != context.DeadlineExceeded {
			errChan <- fmt.Errorf("expected context.DeadlineExceeded, got %v", err)
		} else {
			errChan <- nil
		}
		close(errChan)
		wg.Done()
	}()

	wg.Wait()
	if err := <-errChan; err != nil {
		t.Fatal(err.Error())
	}

	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected elapsed time to be less than 100ms, got %v", elapsed)
	}
}
