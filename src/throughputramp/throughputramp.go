package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"throughputramp/aggregator"
	"throughputramp/data"
	"throughputramp/plotgen"
	"throughputramp/uploader"
)

var (
	numRequests      = flag.Int("n", 1000, "number of requests to send")
	proxy            = flag.String("x", "", "proxy for request")
	interval         = flag.Int("i", 1, "interval in seconds to average throughput")
	threadRateLimit  = flag.Int("q", 0, "thread rate limit")
	lowerConcurrency = flag.Int("lower-concurrency", 1, "Starting concurrency value")
	upperConcurrency = flag.Int("upper-concurrency", 30, "Ending concurrency value")
	concurrencyStep  = flag.Int("concurrency-step", 1, "Concurrency increase per run")
	s3Endpoint       = flag.String("s3-endpoint", "", "The endpoint for the S3 service to which plots will be uploaded.")
	s3Region         = flag.String("s3-region", "", "The region for the S3 service to which plots will be uploaded. If provided, endpoint is ignored.")
	bucketName       = flag.String("bucket-name", "", "Name of the bucket to which plots will be uploaded.")
	accessKeyID      = flag.String("access-key-id", "", "AccessKeyID for the S3 service.")
	secretAccessKey  = flag.String("secret-access-key", "", "SecretAccessKey for the S3 service.")
	comparisonFile   = flag.String("comparison-file", "", "CSV file containing data to be used for comparison with the generated plot.")
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit()
	}

	s3Config := &uploader.Config{
		Endpoint:        *s3Endpoint,
		AwsRegion:       *s3Region,
		BucketName:      *bucketName,
		AccessKeyID:     *accessKeyID,
		SecretAccessKey: *secretAccessKey,
	}
	err := s3Config.Validate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "s3 config error: %s\n", err.Error())
		usageAndExit()
	}

	url := flag.Args()[0]

	start := *lowerConcurrency
	end := *upperConcurrency
	step := *concurrencyStep

	var dataPoints []*data.Point
	for i := start; i <= end; i += step {
		points, benchmarkErr := runBenchmark(url, *proxy, *numRequests, i, *threadRateLimit)
		if benchmarkErr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", benchmarkErr.Error())
			os.Exit(1)
		}

		dataPoints = append(dataPoints, points...)
	}

	ag := aggregator.New(dataPoints, time.Duration(*interval)*time.Second)
	report := ag.Data()

	filename := time.Now().UTC().Format(time.RFC3339)

	csvData := report.GenerateCSV()
	loc, err := uploader.Upload(s3Config, bytes.NewBuffer(csvData), filename+".csv", false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uploading to s3 error: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "csv uploaded to %s\n", loc)

	fmt.Fprintln(os.Stderr, "Generating plot from csv data")
	plotBuffer, err := plotgen.Generate(filename, csvData, *comparisonFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate plot: %s", err.Error())
		os.Exit(1)
	}

	loc, err = uploader.Upload(s3Config, plotBuffer, filename+".png", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uploading to s3 error: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "png uploaded to %s\n", loc)
}

func runBenchmark(url, proxy string, numRequests, concurrentRequests, rateLimit int) ([]*data.Point, error) {
	fmt.Fprintf(os.Stderr, "Running benchmark with %d requests, %d concurrency, and %d rate limit\n", numRequests, concurrentRequests, rateLimit)
	args := []string{
		"-x", proxy,
		"-n", strconv.Itoa(numRequests),
		"-c", strconv.Itoa(concurrentRequests),
		"-q", strconv.Itoa(rateLimit),
		"-o", "csv",
		url,
	}

	heyData, err := exec.Command("hey", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("hey error: %s\nData:\n%s", err.Error(), string(heyData))
	}

	dataPoints, err := data.Parse(string(heyData))
	if err != nil {
		return nil, fmt.Errorf("parse error: %s\nData:\n%s", err.Error(), string(heyData))
	}

	return dataPoints, nil
}

func usageAndExit() {
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}
