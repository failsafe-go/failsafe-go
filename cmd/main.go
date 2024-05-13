package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
)

var breakers []circuitbreaker.CircuitBreaker[any]
var lists [][]string

func main() {
	go func() {
		http.ListenAndServe("localhost:8080", nil)
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go makeBreakers(&wg)
	wg.Wait()
}

func makeBreakers(wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < 1000; i++ {
		breaker := circuitbreaker.Builder[any]().
			WithDelay(time.Minute).
			WithFailureThresholdPeriod(10, time.Minute).Build()
		breakers = append(breakers, breaker)

		tst := make([]string, 30)
		lists = append(lists, tst)
	}
	fmt.Println(len(breakers))
	time.Sleep(10000000 * time.Minute)
}
