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
		return false, TestResult{Success: false, Node:node,Url:url,Reference: ref, Status: resp.StatusCode}
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

func doTest() {
	ref := postTest(10)

	var wg sync.WaitGroup
	wg.Add(70)

	var outT []int
	var outF []int
	var results []TestResult

	for i := 0; i < 70; i++ {
	    go func(i int) {
	        defer wg.Done()
	        o, r := getTest(ref, i)

	        results = append(results, r)

	        if o == false {
		        outF = append(outF, i)	        	
	        }

	        if o == true {
    	        outT = append(outT, i)
	        }
	    }(i)
	}

	wg.Wait()
	fmt.Println(true, len(outT), outT)
	fmt.Println(false, len(outF), outF)

	var missing []int
	for i :=0; i < 70; i++ {
		if arrayContains(outT,i) == false {
			missing = append(missing, i)
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Node < results[j].Node })

	fmt.Println("missing", len(missing), missing)

	for i := 0; i < len(results); i++ {
		fmt.Println(results[i])		
	}

	fmt.Println("\n\n\n")


}

func main(){
	// var wg sync.WaitGroup
	// wg.Add(10)
	for i := 0; i <= 40; i++ {
	// 	go func(i int) {
	// 		time.Sleep(5 * time.Second)
	// 		defer wg.Done()
			doTest()
	// 	}(i)
	}
	// wg.Wait()
}