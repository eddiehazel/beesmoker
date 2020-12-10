package main

import (
	"fmt"
)

func main(){

	outT := [5]int{10, 20, 30, 40, 50}

	var missing []int
	for i :=0; i < 70; i++ {
		if arrayContains(outT,i) == false {
			missing = append(missing, i)
		}
	}

	fmt.Println("missing", len(outF), outF)

}