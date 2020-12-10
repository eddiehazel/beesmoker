package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"bytes"
	"crypto/rand"
	"net/http"
	"encoding/json"
	"sync"
	"time"
	"path/filepath"
	"sort"
)

const (
	postTo = "http://localhost:1733/bytes"
	postType = "application/octet-stream"
	tmpFolder = "tmp"
	getFromTemp = "https://bee-%d.gateway.ethswarm.org/bytes/%s"
	postSize = 10
)


func postTest(size int64) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(io.LimitReader(rand.Reader, size))

	resp, err := http.Post(postTo, postType, buf)
	if err != nil {
		panic(err.Error())
	}

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

	fmt.Println(r.Reference)

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

	url := fmt.Sprintf(getFromTemp, node, ref);

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	time.Sleep(1 * time.Second)

	resp, err := client.Get(url)
	if err != nil {
		fmt.Println(fmt.Sprintf(getFromTemp, node, ref))
		fmt.Println("error:", err)
		return false, TestResult{Success: false, Node:node, Url:url, Reference: ref, Status: 0}
	}

	suc := resp.StatusCode == 200
	// fmt.Println("got:", resp.StatusCode	)

	testResult := TestResult{Success: true,Node:node,Url:url,Reference: ref, Status: resp.StatusCode}

	if suc != true {
		fmt.Println(url)
		fmt.Println(node, resp.StatusCode)
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

func doTest(retryChannel chan TestResult) {
	ref := postTest(postSize)

	var results []TestResult

	resultsChannel := make(chan TestResult)  

	for i := 0; i < 70; i++ {
	    go func(i int) {
	        _, r := getTest(ref, i)
    	    resultsChannel <- r
	    }(i)
	}

	
	successful := 0

    for {
        v := <-resultsChannel
        results = append(results, v)
        if v.Success == true {
        	successful++
        }
        if v.Success != true {
        	retryChannel <- v
        }
        if len(results) == 70 {
            close(resultsChannel)
            break
        }
    }

    fmt.Println("Successful: ", successful)

	sort.Slice(results, func(i, j int) bool { return results[i].Node < results[j].Node })

	// for i := 0; i < len(results); i++ {
	// 	if results[i].Success != true {
	// 		fmt.Println("Failed:", results[i])					
	// 	}
	// }

	// for i := 0; i < len(results); i++ {
	// 	if results[i].Success == true {
	// 		fmt.Println(results[i])					
	// 	}
	// }

}

func main(){
	retryChannel := make(chan TestResult)  

	var refsToRetry []TestResult
    for r := range retryChannel {
    	fmt.Println("Failed ",r)
        refsToRetry = append(refsToRetry, r)
    }

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i <= 9; i++ {
		time.Sleep(1 * time.Second)
		go func(i int) {
			defer wg.Done()
			doTest(retryChannel)
		}(i)
	}

	wg.Wait()
    close(retryChannel)

    fmt.Println("retry",refsToRetry)


}