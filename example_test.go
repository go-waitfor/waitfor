package waitfor_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/go-waitfor/waitfor/v2"
)

func ExampleRunner_Check() {
	runner, err := waitfor.New(waitfor.ResourceConfig{
		Scheme: []string{"ready"},
		Factory: func(*url.URL) (waitfor.Resource, error) {
			return waitfor.ResourceFunc(func(context.Context) error { return nil }), nil
		},
	})
	if err != nil {
		panic(err)
	}
	results, err := runner.Check(context.Background(), []string{"ready://a", "ready://b"},
		waitfor.WithInterval(500*time.Millisecond),
		waitfor.WithMaxInterval(5*time.Second),
		waitfor.WithMaxElapsedTime(30*time.Second),
	)
	if err != nil {
		panic(err)
	}
	for _, result := range results {
		fmt.Printf("%s attempts=%d ready=%t\n", result.Resource, result.Attempts, result.Err == nil)
	}
	// Output:
	// ready://a attempts=1 ready=true
	// ready://b attempts=1 ready=true
}

func ExampleWaitError() {
	notReady := errors.New("not ready")
	runner, err := waitfor.New(waitfor.ResourceConfig{
		Scheme: []string{"custom"},
		Factory: func(*url.URL) (waitfor.Resource, error) {
			return waitfor.ResourceFunc(func(context.Context) error { return notReady }), nil
		},
	})
	if err != nil {
		panic(err)
	}
	err = runner.Test(context.Background(), []string{"custom://service"}, waitfor.WithAttempts(1))
	fmt.Println(errors.Is(err, waitfor.ErrWait), errors.Is(err, notReady))
	var failures *waitfor.WaitError
	if errors.As(err, &failures) {
		fmt.Println(failures.Errors[0].Resource)
	}
	// Output:
	// true true
	// custom://service
}

func ExampleWithAttemptObserver() {
	runner, err := waitfor.New(waitfor.ResourceConfig{
		Scheme: []string{"custom"},
		Factory: func(*url.URL) (waitfor.Resource, error) {
			calls := 0
			return waitfor.ResourceFunc(func(context.Context) error {
				calls++
				if calls == 1 {
					return errors.New("not ready")
				}
				return nil
			}), nil
		},
	})
	if err != nil {
		panic(err)
	}
	err = runner.Test(context.Background(), []string{"custom://service"},
		waitfor.WithAttempts(2),
		waitfor.WithInterval(time.Millisecond),
		waitfor.WithRandomizationFactor(0),
		waitfor.WithAttemptObserver(func(a waitfor.Attempt) {
			fmt.Printf("%s attempt=%d next=%s\n", a.Resource, a.Number, a.Next)
		}),
	)
	fmt.Println(err)
	// Output:
	// custom://service attempt=1 next=1ms
	// <nil>
}
