//  coding: utf-8
// gotest1:TestCreep.go by Ricky Seltzer rickyseltzer@gmail.com.  Version 1.0 on 2013-06-04
// This started as exercise 70 (the web crawler) on the Tour of Go: http://tour.golang.org/#70

/*
 * TESTING:
 * Todo: Need to test links to anchors on the same page: Don't go follow.
 * Todo: Code mixes t.Logf with log.Printf, t.Log with log.Println.  Pick one set.
 *
 * WALK/CREEP ITSELF:
 * Todo: call-back with each url and enough info to build a tree of urls.  Build it on the
 *      map of urls for de-duping?
 * Todo: Do we really need the boolean in the map?
 * Todo: honor robots.txt
 * Todo: collect meta tags.
 * Todo: Perhaps even store a summary of web pages for later searching?
 *       (Nobody would ever have a use for such a thing, though.)
 * Todo: Do Not Crawl in the Dust: http://www2007.org/paper194.php
 *       URL normalization (for has-been-seen) en.wikipedia.org/wiki/URL_normalization
 *
 */

// Package creep tester: test the web crawler.
package creep

import (
	"fmt"
	"log"
	"runtime"
	//"strings"
	"testing"
	"time"
)

var urlCount int = 0
var urlLength int = 0
var sumElapsed time.Duration = 0

var statusCodeCounts [601]int // store counts of status code appearances.  Mostly 200s, some 404s...

// Independent diagnostic loop that periodically dumps out information of interest.
func monitor(t *testing.T) {
	var sleeper int = 2 // number of seconds to nap.  To start.
	var counter int = 0
	var avgLen float64 = 0.0
	var avgDur time.Duration = 0

	for {
		synched.Lock() // Write lock shared data.
		// Just copy the struct??
		qC := synched.queueCnt
		rJ := synched.rejectCnt
		uC := urlCount
		uL := urlLength
		synched.Unlock() // Write unlock shared data.

		if uC > 0 {
			avgLen = float64(uL) / float64(uC)
			avgDur = sumElapsed / time.Duration(uC)
		}

		strange := (qC - reqChanCapacity) == (rJ + goingCount) // true when it hangs sometimes.  Really odd...
		straTF := boolTF(strange)
		rr := fmt.Sprintf("%6d:%4d.", len(reqChan), len(respChan))
		log.Printf("Gos: %3d, req:resp %s, Urls %4d, enQ %4d, avgLen %2.2f, avgDur %12v. stra %s, ml %4d\n",
			runtime.NumGoroutine(), rr, urlCount, qC, avgLen, avgDur, straTF, mapLength())
		showStatusOnLog()
		counter++
		if 10 > counter {
			sleeper += 2
		} else if 20 > counter {
			sleeper += 5
		}
		time.Sleep(time.Duration(sleeper) * time.Second)
	}
}

/* The testing package calls TestCreep since name is 'TestX.*' where X is a capital letter.
 * So far there are 2 kinds of tests: Urls that are expected to succeed (Urls that we expect
 * to be Get() okay), and urls that cannot succeed, so an error return is required.
 */
func TestCreep(t *testing.T) {
	jobData := LoadJobData("testicann.json") // Load test data into struct jobData. See loadtestfile.go

	go monitor(t)

	startTime := time.Now()
	for i := 0; i < len(jobData.Tests); i++ { // Once for each test in the jobData
		eachUt := jobData.Tests[i]
		log.Println("")
		testnameDisplay := "'" + eachUt.Testname + "'"
		log.Printf("Test %12s has: Maxurls %3d, Gomaxprocs %2d, MaxGoRoutines %3d, ExpectFail %v, JustOneDomain %s, %2d urls:",
			testnameDisplay, eachUt.Maxurls, eachUt.Gomaxprocs, eachUt.MaxGoRoutines,
			eachUt.ExpectFail, boolTF(eachUt.JustOneDomain), len(eachUt.Urls))

		runtime.GOMAXPROCS(eachUt.Gomaxprocs)

		for urlNum := 0; urlNum < len(eachUt.Urls); urlNum++ { // Once for each url in the current test.
			log.Printf("TestUrl # %3d: %s\n", urlNum, eachUt.Urls[urlNum])
		}

		doTest(t, &eachUt)
	}
	log.Printf("Test ending after %v simulating %v\n", time.Since(startTime), sumElapsed)
}

/*
Start off the Creeper and then loops on result of the response channel.
*/
func doTest(t *testing.T, pEachUt *JobData) {

	urls := pEachUt.Urls
	expectFail := pEachUt.ExpectFail
	maxurls := pEachUt.Maxurls
	maxGoRoutines := pEachUt.MaxGoRoutines
	testname := pEachUt.Testname

	respChan := CreepWebSites(urls, maxurls, maxGoRoutines, pEachUt.JustOneDomain) // Call the software under test (SUT)

OnceForEachResponse:
	for {
		result, notDoneYet := <-respChan
		if !notDoneYet { // if not notDoneYet, then we are done. Sorry for the double negative...
			// next time I'll use a range.
			//Channel has been closed by waitGroup, we should be all done by now.
			testnameDisplay := "'" + testname + "'"
			t.Logf("Test Closed %12s: %4d urls Fetched, %4d dupes. Elapsed: %v, len (reqQ) %3d, resps: %3d\n\n",
				testnameDisplay, synched.urlsFetched, synched.dupsStopped, sumElapsed, len(reqChan), urlCount)
			showStatusOnLog()
			showSummary(t)
			return
		}

		if (nil != result) && ("DONE" == result.Url) {
			//Channel has been closed, we should be all done by now.
			testnameDisplay := "'" + testname + "'"
			t.Logf("Test Done %12s: %4d urls Fetched, %4d dupes. Elapsed: %v, len (reqQ) %3d, resps: %3d\n\n",
				testnameDisplay, synched.urlsFetched, synched.dupsStopped, sumElapsed, len(reqChan), urlCount)
			showStatusOnLog()
			showSummary(t)
			return
		}

		if nil == result {
			t.Fatalf("UnExpected NIL result\n")
			return
		}

		sumElapsed += result.ElapsedTime
		urlCount++                   // Bug: Not properly synchronized.  Not that important, either.
		urlLength += len(result.Url) // Bug: Same synchro problem.

		if nil != result.HttpResponse {
			sc := result.HttpResponse.StatusCode
			if (0 <= sc) && ((len(statusCodeCounts) - 1) > sc) {
				statusCodeCounts[sc]++
			} else {
				statusCodeCounts[len(statusCodeCounts)-1]++ // Count Invalid status codes.
			}
		}
		if expectFail {
			if nil == result.Err { // no fail.  Only the package tester uses this facility.
				t.Errorf("\n ==>> ERR (did not expect success): for %s\n", result.Url)
				// should we print stuff out if we got a good result on a fake url??
			}
			continue OnceForEachResponse
		} else {
			var statuscode int = -1
			if (nil != result) && (nil != result.HttpResponse) {
				statuscode = result.HttpResponse.StatusCode
			}

			if nil != result.Err { // did fail on a 'good' url, not expected to fail.
				// Bug: Errors out in the web can produce an 'error', not necessarily a program error.
				t.Errorf("\n ==>> ERR (did not expect error): [%3d] %v\n", statuscode, result.Err)
				continue OnceForEachResponse
			}
			if statuscode == -1 {
				t.Logf("Test got result %d on '%s'\n", statuscode, result.Url)
			}
		}
	} // end for

}

func showSummary(t *testing.T) {
	var total = 0
	for sc, sCount := range statusCodeCounts {
		if 0 < sCount {
			total += sCount
			t.Logf("StatusCode %3d:  %6d\n", sc, sCount)
		}
	}
	t.Logf("StatusCode Total: %5d\n", total)
}
