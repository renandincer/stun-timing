package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pion/stun"
	"github.com/schollz/progressbar/v3"
)

type config struct {
	stunHost string
	runCount int
	timeout  time.Duration
}

type result struct {
	time int64
	err  error
}

func main() {
	cfg := parseFlags()

	results, err := runSTUNRequests(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	printResults(results)
	printASCIIHistogram(results)
}

func parseFlags() config {
	stunHost := flag.String("host", "stun.cloudflare.com:3478", "STUN server hostname")
	runCount := flag.Int("runs", 1, "Number of times to run the STUN request")
	timeout := flag.Duration("timeout", 5*time.Second, "Timeout for each STUN request")
	flag.Parse()

	return config{
		stunHost: *stunHost,
		runCount: *runCount,
		timeout:  *timeout,
	}
}

func runSTUNRequests(cfg config) ([]result, error) {
	u, err := stun.ParseURI("stun:" + cfg.stunHost)
	if err != nil {
		return nil, fmt.Errorf("failed to parse STUN URI: %w", err)
	}

	c, err := stun.DialURI(u, &stun.DialConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to dial STUN server: %w", err)
	}
	defer c.Close()

	results := make([]result, cfg.runCount)

	fmt.Println("Starting STUN requests...")
	bar := progressbar.Default(int64(cfg.runCount))

	for i := 0; i < cfg.runCount; i++ {
		message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

		start := time.Now()
		err := c.Do(message, func(res stun.Event) {
			if res.Error != nil {
				return
			}

			var xorAddr stun.XORMappedAddress
			if err := xorAddr.GetFrom(res.Message); err != nil {
				return
			}

			if i == 0 {
				fmt.Printf("\nYour IP is: %s\n", xorAddr.IP)
			}
		})

		elapsed := time.Since(start).Microseconds()
		results[i] = result{time: elapsed, err: err}

		bar.Add(1)
	}

	fmt.Println() // New line after progress bar
	return results, nil
}

func printResults(results []result) {
	var successfulTimes []int64
	var errorCount int

	for i, r := range results {
		if r.err != nil {
			errorCount++
			continue
		}

		successfulTimes = append(successfulTimes, r.time)

		if i == 0 {
			fmt.Printf("First request time: %d μs\n", r.time)
		}
	}

	if len(successfulTimes) == 0 {
		fmt.Println("No successful requests")
		return
	}

	sort.Slice(successfulTimes, func(i, j int) bool { return successfulTimes[i] < successfulTimes[j] })

	fmt.Println("\nResults:")
	fmt.Printf("Successful requests: %d\n", len(successfulTimes))
	fmt.Printf("Failed requests: %d\n\n", errorCount)

	fmt.Println("┌───────┬───────────┐")
	fmt.Println("│ %tile │ Time (μs) │")
	fmt.Println("├───────┼───────────┤")
	fmt.Printf("│  p0   │ %9d │\n", successfulTimes[0])
	fmt.Printf("│  p25  │ %9d │\n", percentile(successfulTimes, 25))
	fmt.Printf("│  p50  │ %9d │\n", percentile(successfulTimes, 50))
	fmt.Printf("│  p75  │ %9d │\n", percentile(successfulTimes, 75))
	fmt.Printf("│ p100  │ %9d │\n", successfulTimes[len(successfulTimes)-1])
	fmt.Println("└───────┴───────────┘")
}

func percentile(sorted []int64, p int) int64 {
	index := int(math.Round(float64(len(sorted)-1) * float64(p) / 100))
	return sorted[index]
}

func printASCIIHistogram(results []result) {
	var successfulTimes []int64
	for _, r := range results {
		if r.err == nil {
			successfulTimes = append(successfulTimes, r.time)
		}
	}

	if len(successfulTimes) == 0 {
		return
	}

	// Determine min and max times
	minTime, maxTime := successfulTimes[0], successfulTimes[0]
	for _, t := range successfulTimes {
		if t < minTime {
			minTime = t
		}
		if t > maxTime {
			maxTime = t
		}
	}

	// Create buckets
	numBuckets := 20
	bucketSize := float64(maxTime-minTime) / float64(numBuckets)
	buckets := make([]int, numBuckets)

	for _, t := range successfulTimes {
		bucket := int(float64(t-minTime) / bucketSize)
		if bucket == numBuckets {
			bucket--
		}
		buckets[bucket]++
	}

	// Find max bucket count for scaling
	maxCount := 0
	for _, count := range buckets {
		if count > maxCount {
			maxCount = count
		}
	}

	// Print histogram
	fmt.Println("\nLatency Distribution (μs):")
	for i, count := range buckets {
		start := int64(float64(i)*bucketSize) + minTime
		end := int64(float64(i+1)*bucketSize) + minTime
		bar := strings.Repeat("█", count*40/maxCount)
		fmt.Printf("%6d - %6d | %-40s | %d\n", start, end, bar, count)
	}
}
