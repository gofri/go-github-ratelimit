package github_ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_SleepWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	wg := &sync.WaitGroup{}
	wg.Add(1)

	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 1*time.Second)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		wg.Done()
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	wg.Wait()
	elapsed := time.Since(start)
	assert.LessOrEqual(t, elapsed, 100*time.Millisecond)
}

func Test_SleepWithContext(t *testing.T) {
	ctx := context.Background()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 50*time.Millisecond)
		assert.NoError(t, err)
		wg.Done()
	}()

	wg.Wait()
	elapsed := time.Since(start)
	assert.LessOrEqual(t, elapsed, 100*time.Millisecond)
}

func Test_SleepWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	start := time.Now()
	go func() {
		err := sleepWithContext(ctx, 1*time.Second)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		wg.Done()
	}()

	wg.Wait()
	elapsed := time.Since(start)
	assert.LessOrEqual(t, elapsed, 100*time.Millisecond)
}
