//  coding: utf-8
// loadtestfile.go by Ricky Seltzer rickyseltzer@gmail.com.  Version 1.0 on 2013-06-11

package creep

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

type JobData struct {
	Testname      string
	Maxurls       int
	MaxGoRoutines int
	Gomaxprocs    int
	ExpectFail    bool
	JustOneDomain bool
	Urls          []string
}
type JobDataArray struct {
	Tests []JobData
	// The json pkg will load into this array as many structs as there are in the file.
}

var JobDescription JobDataArray

func LoadJobData(filename string) *JobDataArray {
	jsonD, err := ioutil.ReadFile(filename) // jsonD is []byte
	if nil != err {
		log.Fatal(err)
	}
	//log.Printf("Read %d bytes from file %s\n", len(jsonD), filename)

	jerr := json.Unmarshal(jsonD, &JobDescription) // Convert json data to Go structs.
	if nil != jerr {
		log.Fatal(jerr)
	}
	return &JobDescription
}
