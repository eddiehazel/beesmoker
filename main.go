// TROJAN TESTS!

package main

import (
	"sync"
	"fmt"
	"io"
	"io/ioutil"
	"bytes"
	"crypto/rand"
	"net/http"
	"encoding/json"
	"time"
	"path/filepath"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"strconv"
)

// const (
// 	concurrentUploads = true //as long as the pusher keeps up with the posts, it's fine and much quicker to do this, for bigger uploads, use non-concurrent uploads ;)
// 	postTo = "http://localhost:1733/bytes"
// 	getTagStatusTemplate = "http://localhost:1733/tags/%s"
// 	postType = "application/octet-stream"
// 	tmpFolder = "tmp"
// 	getFromTemplate = "https://bee-%d.gateway.staging.ethswarm.org/bytes/%s"
// 	maxNode = 19 //presuming they start at 0

// 	postSize = 10
// 	batchSize =  5000
// 	getTestTimoutSecs = 100
// 	sleepBetweenBatchMs = 100
// 	sleepBetweenRetryMs = 5000
// 	maxRetryAttempts = 3
// )

const (
	concurrentUploads = false
	postTo = "http://localhost:1633/files"
	getTagStatusTemplate = "http://localhost:1633/tags/%s"
	promGateway = "http://localhost:9091"
	postType = "application/octet-stream"
	tmpFolder = "tmp"
	getFromTemplate = "https://bee-%d.gateway.ethswarm.org/bytes/%s"
	maxNode = 69 //presuming they start at 0

	postSize = 1.5 * 1000 * 1000
	batchSize =  10
	getTestTimoutSecs = 100
	timeBeforeGetSecs = 40
	sleepBetweenBatchMs = 300
	sleepBetweenRetryMs = 1000
	maxRetryAttempts = 1
)

var (
	timestamp string
	responseDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "sig",
		Subsystem: "smoketests",
		Name:      "get_duration_seconds",
		Help:      "Histogram of get durations.",
		Buckets:   []float64{0.1, 0.2, 0.5 , 1, 1.5, 2, 5, 10, 30},
	})
	postedDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "sig",
		Subsystem: "smoketests",
		Name:      "posted_duration_seconds",
		Help:      "Histogram of post durations.",
		Buckets:   []float64{0.1, 0.2, 0.5 , 1, 1.5, 2, 5, 10, 30 , 50, 100, 200},
	})
	syncedDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "sig",
		Subsystem: "smoketests",
		Name:      "sync_duration_seconds",
		Help:      "Histogram of sync durations.",
		Buckets:   []float64{0.1, 0.2, 0.5 , 1, 1.5, 2, 5, 10, 30 , 50, 100, 200},
	})
	syncFailedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	    Namespace: "sig",
	    Subsystem: "smoketests",
	    Name:      "sync_failed",
	    Help:      "Number of abandonded syncs",
	})
	failedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	    Namespace: "sig",
	    Subsystem: "smoketests",
	    Name:      "retries_failed",
	    Help:      "Number of failed even after retries",
	})
	retryGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	    Namespace: "sig",
	    Subsystem: "smoketests",
	    Name:      "retries",
	    Help:      "Number of retries",
	})
)

func obh(mmtx sync.Mutex, metricName string, jobId string, m prometheus.Histogram, start time.Time, ref string){
	mmtx.Lock()
	m.Observe(time.Since(start).Seconds())
	if err := push.New(promGateway, jobId).
		Collector(m).
		// Grouping("ref", ref).
		Grouping("action", metricName).
		Push(); err != nil {
			fmt.Println("Could not push completion time to Pushgateway:", err)
		}
	mmtx.Unlock()
}

// func obg(metricName string, jobId string, m prometheus.Histogram, start time.Time, ref string){
// 	m.Observe(time.Since(start).Seconds())
// 	if err := push.New(promGateway, jobId).
// 		Collector(m).
// 		Grouping("refy", "b").
// 		Push(); err != nil {
// 			fmt.Println("Could not push completion time to Pushgateway:", err)
// 		}
// }

// func obs(metricName string, jobId string, m prometheus.Histogram, start time.Time, ref string){
// 	m.Observe(time.Since(start).Seconds())
// 	if err := push.New(promGateway, jobId).
// 		Collector(m).
// 		Grouping("refz", "c").
// 		Push(); err != nil {
// 			fmt.Println("Could not push completion time to Pushgateway:", err)
// 		}
// }

func obg(metricName string, jobId string, m prometheus.Gauge){
	m.Inc()
	if err := push.New(promGateway, jobId).
		Collector(m).
		Grouping("refa", "x").
		Push(); err != nil {
			fmt.Println("Could not push completion time to Pushgateway:", err)
		}
}


func postTest(mmtx sync.Mutex, i int, size int64) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(io.LimitReader(rand.Reader, size))


	start := time.Now()

	resp, err := http.Post(postTo, postType, buf)
	if err != nil {
		panic(err.Error())
	}

	tagUID := resp.Header["Swarm-Tag-Uid"][0]

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err.Error())
	}

	type Response struct {
	  Reference string
	}
	var r Response
	json.Unmarshal(body, &r)

	fmt.Println("posted", r.Reference)

	obh(mmtx, "postedDuration", timestamp, postedDuration, start, r.Reference)

	attempt := 0
	syncing := true
	for syncing == true {


		resp, err := http.Get(fmt.Sprintf(getTagStatusTemplate, tagUID))
		if err != nil {
			panic(err.Error())
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err.Error())
		}

		type TagResponse struct {
			Total int
			Split int
			Seen int
			Stored int
			Sent int
			Synced int
		}
		var tr TagResponse
		json.Unmarshal(body, &tr)

		if tr.Synced >= tr.Total {

			obh(mmtx, "syncedDuration", timestamp, syncedDuration, start, r.Reference)

			fmt.Println("synced", i, tr)
			syncing = false
		}

		//todo, link this to file size and expected sync time / attempts
		if attempt > 100 {
			fmt.Println("still not synced, abandoning... ", i, tr)
			obg("syncFailedGauge", timestamp, syncFailedGauge)
			syncing = false
		}

		time.Sleep(1000 * time.Millisecond)
		attempt++

	}

	//save file for later investigations
    ioutil.WriteFile(filepath.Join(tmpFolder, r.Reference), buf.Bytes(), 0777)

    if err != nil {
        panic(err.Error())
    }

	return r.Reference
}

type TestResult struct {
  Success bool
  Node int
  Url string
  Reference string
  Status int
}

func getTest(mmtx sync.Mutex, ref string, node int) (bool, TestResult) {

	url := fmt.Sprintf(getFromTemplate, node, ref);

	client := http.Client{
		Timeout: getTestTimoutSecs * time.Second,
	}

	time.Sleep(timeBeforeGetSecs * time.Second)

	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		fmt.Println("error:", err)
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0}
	}

	suc := resp.StatusCode == 200

	testResult := TestResult{Success: true, Node: node, Url: url, Reference: ref, Status: resp.StatusCode}

	if suc != true {
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: resp.StatusCode}
	}

	obh(mmtx, "responseDuration", timestamp, responseDuration, start, ref)

	return suc, testResult
}

func arrayContains(r []int, s int) bool{
	for _, a := range r {
        if a == s {
            return true
        }
    }
    return false
}

func testRun(mmtx sync.Mutex, i int, resultsChannel chan []TestResult, retryChannel chan TestResult, syncDoneChannel chan bool) {

	ref := postTest(mmtx, i, postSize)

    if concurrentUploads == false {
    	syncDoneChannel <- true
	}

	var results []TestResult

	resultChannel := make(chan TestResult)

	for i := 0; i <= maxNode; i++ {
	    go func(i int) {
	        _, r := getTest(mmtx, ref, i)
    	    resultChannel <- r
	    }(i)
	}

	
	successful := 0

	for v := range resultChannel {
        results = append(results, v)
        if v.Success == true {
        	successful++
        }
        if v.Success != true {
        	retryChannel <- v
        }
        if len(results) == maxNode + 1 {
            close(resultChannel)
            break
        }
	}

    resultsChannel <- results
}

func captureResults(resultsChannel chan []TestResult, retryChannel chan TestResult){
	didComplete := 0
	var allResults [][]TestResult
	for {
		res := <-resultsChannel
    	fmt.Println("Completed ", len(res))
        allResults = append(allResults, res)
    	didComplete++
    	fmt.Println(didComplete)
    	if didComplete >= batchSize {
	    	break
    	}
	}
    fmt.Println("cr complete")
}

func doRetry(mmtx sync.Mutex, ref TestResult, retryDoneChannel chan TestResult, attempt int) {
	if attempt >= maxRetryAttempts  {
		fmt.Println("max retry attempts reached, could not retrieve", attempt, ref.Reference, ref.Node)
		retryDoneChannel <- ref

		return
	}
	fmt.Println(ref.Reference, ref.Node, "Trying again... ")
	o, _ref := getTest(mmtx, ref.Reference, ref.Node)
	if o == false {
		nextAttempt := attempt + 1
		timeBeforeRetry := time.Duration( sleepBetweenRetryMs * float64(nextAttempt) ) * time.Millisecond
		fmt.Println("retry failed", nextAttempt, timeBeforeRetry, ref.Reference, ref.Node)
		time.Sleep(timeBeforeRetry)
		doRetry(mmtx, _ref, retryDoneChannel, nextAttempt)
		return
	}else{
		fmt.Println("retry successful", ref.Reference, ref.Node)
		retryDoneChannel <- _ref
	}
}

func captureRetries(refsToRetry *[]TestResult, retryChannel chan TestResult){
	for {
		ret := <-retryChannel
		*refsToRetry = append(*refsToRetry, ret)
	}
}


func main(){

	//print config
	fmt.Println("postTo", postTo)
	fmt.Println("postType", postType)
	fmt.Println("tmpFolder", tmpFolder)
	fmt.Println("getFromTemplate", getFromTemplate)
	fmt.Println("postSize", postSize)
	fmt.Println("batchSize", batchSize)
	fmt.Println("getTestTimoutSecs", getTestTimoutSecs)
	fmt.Println("sleepBetweenBatchMs", sleepBetweenBatchMs)
	fmt.Println("maxNode", maxNode)

	var mmtx sync.Mutex
	timestamp = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	fmt.Println("jobID", timestamp)

	resultsChannel := make(chan []TestResult)  
	retryChannel := make(chan TestResult)  


	//run main batch
	for i := 0; i <= batchSize - 1; i++ {
		fmt.Println(i + 1, "/", batchSize)

		syncDoneChannel := make(chan bool)
		go testRun(mmtx, i, resultsChannel, retryChannel, syncDoneChannel)

		if concurrentUploads == false {
			<- syncDoneChannel
			close(syncDoneChannel)
		}else{
			time.Sleep(sleepBetweenBatchMs * time.Millisecond)			
		}
	}


	//set up retries
	var refsToRetry []TestResult
	refsToRetryP := &refsToRetry
	go captureRetries(refsToRetryP, retryChannel)

	//complete the whole run and capture retries
	captureResults(resultsChannel, retryChannel)

	if len(refsToRetry) > 0 {
		fmt.Println("waiting to start retries", len(refsToRetry))
	}else{
		fmt.Println("Success! 🐝 🐝 🐝 🐝 🐝", timestamp)
		return
	}

	//do retries
	retryDoneChannel := make(chan TestResult)
	for _, ref := range refsToRetry {

		obg("failedGauge", timestamp, failedGauge)

		go doRetry(mmtx, ref, retryDoneChannel, 0)
		time.Sleep(sleepBetweenBatchMs * time.Millisecond)
	}

	var refsFailed []TestResult

	didRetries := 0
    for ref := range retryDoneChannel {
    	fmt.Println(ref, didRetries, len(refsToRetry))
    	if didRetries == len(refsToRetry) - 1 {
    		break
    	}
    	switch ref.Success {
		    case true:
		    	didRetries++
		    case false:
		    	didRetries++
				refsFailed = append(refsFailed, ref)
		}
	}

	close(retryDoneChannel)

	if len(refsFailed) == 0 {
		fmt.Println("retries completed, success! 🐝 🐝 🐝 🐝 🐝", timestamp)
		return
	}

	if len(refsFailed) > 0 {
		fmt.Println("retries completed, still failed: ", len(refsFailed), timestamp)

		for _, failed := range refsFailed {
			obg("failedGauge", timestamp, failedGauge)
			fmt.Println(failed)
		}	
	}

	fmt.Println("retries completed, failed: ", len(refsFailed), timestamp)

}