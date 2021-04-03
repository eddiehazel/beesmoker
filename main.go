// TROJAN TESTS!

package main

import (
	"sync"
	"fmt"
	"io"
	"os"
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
	"sort"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)


const (
	concurrentUploads = false
	postTo = "http://localhost:1633/bytes"
	getTagStatusTemplate = "http://localhost:1633/tags/%s"
	promGateway = ""
	postType = "application/octet-stream"
	tmpFolder = "tmp"
	getFromTemplate = "https://bee-%d.gateway.ethswarm.org/bytes/%s"
	maxNode = 9 //presuming they start at 0
	postSize = 1 * 1000
	maxAttemptsAfterSent = 10	
	batchSize =  1000
	getTestTimoutSecs = 100
	timeBeforeGetSecs = 60
	timeBetweenGetSecs = 2
	sleepBetweenBatchMs = 300
	sleepBetweenRetryMs = 10000
	maxRetryAttempts = 1
	tgChatID = -503582013
)

//staging
// const (
// 	concurrentUploads = false
// 	postTo = "http://localhost:1633/bytes"
// 	getTagStatusTemplate = "http://localhost:1633/tags/%s"
// 	promGateway = ""
// 	postType = "application/octet-stream"
// 	tmpFolder = "tmp"
// 	getFromTemplate = "https://bee-%d.gateway.staging.ethswarm.org/bytes/%s"
// 	maxNode = 19 //presuming they start at 0
// 	postSize = 1000 * 1000 * 0.1
// 	maxAttemptsAfterSent = 10	
// 	batchSize =  100
// 	getTestTimoutSecs = 100
// 	timeBeforeGetSecs = 60
// 	timeBetweenGetSecs = 2
// 	sleepBetweenBatchMs = 300
// 	sleepBetweenRetryMs = 10000
// 	maxRetryAttempts = 5
// 	tgChatID = -503582013
// )

var (
	tgAPI = os.Getenv("TG_API")
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

func sendToBot(msg string){
	bot, err := tgbotapi.NewBotAPI(tgAPI)
	if err != nil {
		panic(err.Error())
	}

	m := tgbotapi.NewMessage(tgChatID, msg)
	a,b := bot.Send(m)
	fmt.Println(a,b)
}

func obh(mmtx sync.Mutex, metricName string, jobId string, m prometheus.Histogram, start time.Time, ref string){
	if promGateway == ""{
		return
	}
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


func obg(metricName string, jobId string, m prometheus.Gauge){
	if promGateway == ""{
		return
	}
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

	fmt.Println(resp)

	tagUID := resp.Header["Swarm-Tag"][0]

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

	attemptAfterSent := 0
	syncing := true
	lastSynced := 0
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
			Processed int
			Synced int
		}

		var tr TagResponse
		json.Unmarshal(body, &tr)
		fmt.Println("syncing", r.Reference, i, tr)

		//just waiting for sent not sync
		if tr.Synced >= tr.Total {

			obh(mmtx, "syncedDuration", timestamp, syncedDuration, start, r.Reference)

			fmt.Println("synced", r.Reference, i, tr)
			syncing = false
		}


		if lastSynced == tr.Synced && tr.Processed >= tr.Total {
			attemptAfterSent++
		}

		lastSynced = tr.Synced

		if attemptAfterSent > maxAttemptsAfterSent {
			fmt.Println("still not synced, abandoning... ", i, tr)
			obg("syncFailedGauge", timestamp, syncFailedGauge)
			syncing = false
		}

		time.Sleep(1000 * time.Millisecond)

	}

	//save file for later investigations
	ioutil.WriteFile(filepath.Join(tmpFolder, r.Reference), buf.Bytes(), 0777)

	if err != nil {
		panic(err.Error())
	}

	return r.Reference
}

type TestResult struct {
  Success bool `json:"success"`
  Node int `json:"node"`
  Url string `json:"url"`
  Reference string `json:"reference"`
  Status int `json:"status"`
  CompletedTime float64 `json:"completedTime`
}

func getTest(mmtx sync.Mutex, ref string, node int) (bool, TestResult) {

	url := fmt.Sprintf(getFromTemplate, node, ref);

	client := http.Client{
		Timeout: getTestTimoutSecs * time.Second,
	}

	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		fmt.Println("error:", err)
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0, CompletedTime: 0}
	}

	completedTime := time.Since(start).Seconds()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error:", err)
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0, CompletedTime: 0}
	}

	suc := resp.StatusCode == 200

	if suc != true {
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: resp.StatusCode}
	}

	if len(body) != postSize {
		fmt.Println("error: retrieved file is not correct size, retrieved:", len(body), "actual:", int(postSize))
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0, CompletedTime: 0}
	}else{
		fmt.Println("retrieved file with correct size", node)
	}

	testResult := TestResult{Success: true, Node: node, Url: url, Reference: ref, Status: resp.StatusCode, CompletedTime: completedTime}

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

	fmt.Println("waiting", timeBeforeGetSecs, "seconds before attempting to retrieve", ref)
	time.Sleep(timeBeforeGetSecs * time.Second)

	var results []TestResult

	resultChannel := make(chan TestResult)

	for i := 0; i <= maxNode; i++ {
		go func(i int) {
			_, r := getTest(mmtx, ref, i)
			time.Sleep(timeBetweenGetSecs * time.Second)
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

func captureResults(resultsChannel chan []TestResult, retryChannel chan TestResult) [][]TestResult{
	didComplete := 0
	var allResults [][]TestResult
	for {
		res := <-resultsChannel
		fmt.Println("Completed", len(res))
		allResults = append(allResults, res)
		didComplete++
		fmt.Println(didComplete)
		if didComplete >= batchSize {
			break
		}
	}
	fmt.Println("cr complete")
	return allResults
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

func printSortedResults(allResults [][]TestResult) {
	for _, results := range allResults {
		sort.Slice(results, func(i, j int) bool { return results[i].CompletedTime < results[j].CompletedTime })
		fmt.Println(results[0].Url)
		for _, result := range results {
			fmt.Println(result.CompletedTime)
		}
	}
}


func main(){


	// trigger little snitch warning for Dan only
	_, err := http.Get("https://gateway.ethswarm.org")
	if err != nil {
		panic(err.Error())
	}

	//print config

	fmt.Println("concurrentUploads", concurrentUploads)
	fmt.Println("postTo", postTo)
	fmt.Println("getTagStatusTemplate", getTagStatusTemplate)
	fmt.Println("promGateway", promGateway)
	fmt.Println("postType", postType)
	fmt.Println("tmpFolder", tmpFolder)
	fmt.Println("getFromTemplate", getFromTemplate)
	fmt.Println("maxNode", maxNode)
	fmt.Println("postSize", postSize)
	fmt.Println("maxAttemptsAfterSent", maxAttemptsAfterSent)
	fmt.Println("batchSize", batchSize)
	fmt.Println("getTestTimoutSecs", getTestTimoutSecs)
	fmt.Println("timeBeforeGetSecs", timeBeforeGetSecs)
	fmt.Println("sleepBetweenBatchMs", sleepBetweenBatchMs)
	fmt.Println("sleepBetweenRetryMs", sleepBetweenRetryMs)
	fmt.Println("maxRetryAttempts", maxRetryAttempts)

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
	allResults := captureResults(resultsChannel, retryChannel)

	printSortedResults(allResults)

    // ar, err := json.Marshal(allResults)
    // if err != nil {
    //     fmt.Println(err)
    //     return
    // }
    // fmt.Println(string(ar))


    retryCount := len(refsToRetry)

	if len(refsToRetry) > 0 {
		fmt.Println("waiting to start retries", len(refsToRetry))
	}else{
		out := fmt.Sprintf("Success! Completed %v requests! 🐝 🐝 🐝 🐝 🐝 - %v", batchSize * (maxNode+1), timestamp)
		fmt.Println(out)
		sendToBot(out)
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
		out := fmt.Sprintf("Success! Completed %v requests with %v retries! 🐝 🐝 🐝 🐝 🐝 - %v", batchSize * (maxNode+1), retryCount, timestamp)
		fmt.Println(out)
		sendToBot(out)
		return
	}

	if len(refsFailed) > 0 {
		fmt.Println("retries completed, still failed: ", len(refsFailed), timestamp, "😟🙈🌤")

		for _, failed := range refsFailed {
			obg("failedGauge", timestamp, failedGauge)
			fmt.Println(failed)
		}	
	}


	out := fmt.Sprintf("🙈 Completed %v/%v requests with %v retries! 🐝 🌥 ♥️ - %v", batchSize * (maxNode+1) - len(refsFailed), batchSize * (maxNode+1), retryCount, timestamp)
	fmt.Println(out)
	sendToBot(out)
	return
	fmt.Println("retries completed, failed: ", len(refsFailed), "of", batchSize * (maxNode+1), "run:", timestamp, "😟🙈🌤")

}
