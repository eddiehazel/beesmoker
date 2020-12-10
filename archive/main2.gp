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
)


func postTest(size int64) string {

	type Response struct {
	  Reference string
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(io.LimitReader(rand.Reader, size))

	resp, err := http.Post("http://localhost:1733/bytes", "application/octet-stream", buf)
	if err != nil {
		panic(err.Error())
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err.Error())
	}

	var r Response
	json.Unmarshal(body, &r)

	time.Sleep(5 * time.Second)

	return r.Reference
}

func getTest(ref string, node int){

	url := fmt.Sprintf("https://bee-%d.gateway.ethswarm.org/bytes/%s", node, ref);


	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(fmt.Sprintf("https://bee-%d.gateway.ethswarm.org/bytes/%s", node))
		fmt.Println("error:", err)
		return
	}

	fmt.Println(node, resp.StatusCode)
}

func doTest() {
	ref := postTest(10)

	var wg sync.WaitGroup
	wg.Add(70)

	// fmt.Println("Running for loop…")
	for i := 0; i < 70; i++ {
	    go func(i int) {
	        defer wg.Done()
	        // fmt.Println(fmt.Sprintf("Running for loop… %d", i))
	        getTest(ref, i)
	    }(i)
	}

	wg.Wait()
}

func main(){
	for i := 0; i < 1000; i++ {
		doTest()
	}
}