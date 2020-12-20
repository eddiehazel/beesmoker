// TROJAN TESTS!

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"bytes"
	"crypto/rand"
	"net/http"
	"encoding/json"
	"time"
	"path/filepath"
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
	concurrentUploads = true
	postTo = "http://localhost:1633/bytes"
	getTagStatusTemplate = "http://localhost:1633/tags/%s"
	postType = "application/octet-stream"
	tmpFolder = "tmp"
	getFromTemplate = "https://bee-%d.gateway.ethswarm.org/bytes/%s"
	maxNode = 69 //presuming they start at 0

	postSize = 1000
	batchSize =  1
	getTestTimoutSecs = 100
	sleepBetweenBatchMs = 300
	sleepBetweenRetryMs = 5000
	maxRetryAttempts = 3
)

func postTest(size int64) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(io.LimitReader(rand.Reader, size))

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


	synced := false
	for synced == false {

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
		var r TagResponse
		json.Unmarshal(body, &r)

		if r.Synced >= r.Total {
			// fmt.Println("synced", r)
			synced = true
		}

		time.Sleep(2 * time.Second)

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

func getTest(ref string, node int) (bool, TestResult) {

	url := fmt.Sprintf(getFromTemplate, node, ref);

	client := http.Client{
		Timeout: getTestTimoutSecs * time.Second,
	}


	resp, err := client.Get(url)
	if err != nil {
		// fmt.Println(fmt.Sprintf(getFromTemplate, node, ref))
		fmt.Println("error:", err)
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0}
	}

	suc := resp.StatusCode == 200

	testResult := TestResult{Success: true,Node:node,Url:url,Reference: ref, Status: resp.StatusCode}

	if suc != true {
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: resp.StatusCode}
	}

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

func testRun(resultsChannel chan []TestResult, retryChannel chan TestResult, syncDoneChannel chan bool) {
	ref := postTest(postSize)

	var results []TestResult

	resultChannel := make(chan TestResult)

	for i := 0; i <= maxNode; i++ {
	    go func(i int) {
	        _, r := getTest(ref, i)
    	    resultChannel <- r
	    }(i)
	}

	
	successful := 0

    for {
        v := <-resultChannel
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

    if concurrentUploads == false {
    	syncDoneChannel <- true
	}


    // fmt.Println("success", successful, ref)

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
    	if didComplete >= batchSize {
	    	break
    	}
	}
    fmt.Println("cr complete")
}

func doRetry(ref TestResult, retryDoneChannel chan TestResult, attempt int) {
	if attempt >= maxRetryAttempts  {
		fmt.Println("max retry attempts reached, could not retrieve", attempt, ref.Reference, ref.Node)
		retryDoneChannel <- ref

		return
	}
	fmt.Println(ref.Reference, ref.Node, "Trying again... ")
	o, _ := getTest(ref.Reference, ref.Node)
	// fmt.Println(ret)
	if o == false {
		nextAttempt := attempt + 1
		timeBeforeRetry := time.Duration( sleepBetweenRetryMs * float64(nextAttempt) ) * time.Millisecond
		fmt.Println("retry failed", nextAttempt, timeBeforeRetry, ref.Reference, ref.Node)
		time.Sleep(timeBeforeRetry)
		doRetry(ref, retryDoneChannel, nextAttempt)
	}else{
		fmt.Println("retry successful", ref.Reference, ref.Node)
	}
	retryDoneChannel <- ref
}

func captureRetries(refsToRetry *[]TestResult, retryChannel chan TestResult){
	for {
		ret := <-retryChannel
		// fmt.Println(ret)
		*refsToRetry = append(*refsToRetry, ret)
	}
}



func main(){

	fmt.Println("postTo", postTo)
	fmt.Println("postType", postType)
	fmt.Println("tmpFolder", tmpFolder)
	fmt.Println("getFromTemplate", getFromTemplate)
	fmt.Println("postSize", postSize)
	fmt.Println("batchSize", batchSize)
	fmt.Println("getTestTimoutSecs", getTestTimoutSecs)
	fmt.Println("sleepBetweenBatchMs", sleepBetweenBatchMs)
	fmt.Println("maxNode", maxNode)


	resultsChannel := make(chan []TestResult)  
	retryChannel := make(chan TestResult)  

	for i := 0; i <= batchSize - 1; i++ {
		fmt.Println(i + 1, "/", batchSize)

		syncDoneChannel := make(chan bool)
		go testRun(resultsChannel, retryChannel, syncDoneChannel)
		if concurrentUploads == false {
			time.Sleep(sleepBetweenBatchMs * time.Millisecond)
			<- syncDoneChannel
			close(syncDoneChannel)
		}else{
			time.Sleep(sleepBetweenBatchMs * time.Millisecond)			
		}
	}


	var refsToRetry []TestResult
	refsToRetryP := &refsToRetry
	go captureRetries(refsToRetryP, retryChannel)

	//complete the whole run before starting retries
	captureResults(resultsChannel, retryChannel)

	if len(refsToRetry) > 0 {
		fmt.Println("waiting to start retries", len(refsToRetry))
	}else{
		fmt.Println("Success! 🐝 🐝 🐝 🐝 🐝")
		return
	}


	time.Sleep(1 * time.Second)

	retryDoneChannel := make(chan TestResult)

	for _, ref := range refsToRetry {
		go doRetry(ref, retryDoneChannel, 0)
		time.Sleep(sleepBetweenBatchMs * time.Millisecond)
	}

	var refsFailed []TestResult

	didRetries := 0
    for ref := range retryDoneChannel {
    	fmt.Println(ref, didRetries, len(refsToRetry))
    	if didRetries == len(refsToRetry) {
    		break
    	}
    	switch ref.Success {
		    case true:
		    	fmt.Println("suc")
		    	didRetries++
		    case false:
		    	didRetries++
		    	fmt.Println("f")
				refsFailed = append(refsFailed, ref)
		}
	}

	close(retryDoneChannel)

	fmt.Println(len(refsFailed))

	for failed := range refsFailed {
		fmt.Println(failed)
	}

}