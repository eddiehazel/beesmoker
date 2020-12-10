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
	"sync"
)

const (
	postTo = "http://localhost:1633/bytes"
	getTagStatusTemplate = "http://localhost:1733/tags/%s"
	postType = "application/octet-stream"
	tmpFolder = "tmp"
	getFromTemplate = "https://bee-%d.gateway.ethswarm.org/bytes/%s"
	postSize = 10
	batchSize =  1000
	getTestTimoutSecs = 100
	sleepBetweenBatchMs = 100
	maxNode = 69 //presuming they start at 0
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

		time.Sleep(2 * time.Second)

		if r.Synced >= r.Total {
			fmt.Println("synced", r)
			synced = true
		}
	}

	//just assume this is enough time for network to sync for now
	time.Sleep(1 * time.Second)

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

func testRun(resultsChannel chan []TestResult, retryChannel chan TestResult) {
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

	// sort.Slice(results, func(i, j int) bool { return results[i].Node < results[j].Node })

    fmt.Println("success", successful, ref)

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

func doRetry(ref TestResult) bool {
	fmt.Println(ref.Reference, ref.Node, "Trying again... ")
	o, _ := getTest(ref.Reference, ref.Node)
	fmt.Println(ref.Reference, ref.Node, o)
	return o
}

func captureRetries(retMutex *sync.Mutex, refsToRetry *[]TestResult, retryChannel chan TestResult){
	for {
		ret := <-retryChannel
		fmt.Println(ret)
		// retMutex.Lock()
		*refsToRetry = append(*refsToRetry, ret)
		// retMutex.Unlock()
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
		time.Sleep(sleepBetweenBatchMs * time.Millisecond)
		fmt.Println(i + 1, "/", batchSize)
		go testRun(resultsChannel, retryChannel)
	}


	var retMutex *sync.Mutex
	var refsToRetry []TestResult
	refsToRetryP := &refsToRetry
	go captureRetries(retMutex, refsToRetryP, retryChannel)

	captureResults(resultsChannel, retryChannel)

	fmt.Println("waiting to start retries", len(refsToRetry))
	time.Sleep(10 * time.Second)

	var stillNotWorking []TestResult

	for _, ref := range refsToRetry {
		o := doRetry(ref)
		if o == false {
			stillNotWorking = append(stillNotWorking, ref)
		}
	}

	fmt.Println("stillNotWorking", len(stillNotWorking))
	fmt.Println(stillNotWorking)

}