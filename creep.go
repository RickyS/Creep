//  coding: utf-8
// gotest1:Walk.go by Ricky Seltzer rickyseltzer@gmail.com.  Version 1.0 on 2013-06-04

// Todo: There are too many locks and unlocks in this.  Mostly to keep track of statistics, but also
// to keep the map of beenThereDoneThat, which is the only really good reason.

/*
 Package creep implements a web crawler.  It reads web pages and follows links to the rest of
the web, recursively, ad infinitum, within the limits provided.  We use the term creep to avoid name clashes
with other software called 'walk' and 'crawl'.  I'm thinking of changing it to 'stroll'.
*/
package creep

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	//"strings"
	"sync"
	"time"
)

/*  METHOD:
 * Url requests sent along a request channel.
 * Responses with any errors sent back along a response channel.
 * A pool of worker goroutines is started to recv the requests, process them, and send responses back along
 * the response channel.
 *   The processing is getting the url, searching the body for links, and sending requests for
 * each link along the request channel.  We avoid fetching duplicates by keeping a hash (map) of
 * each url fetched.
 */
/*
 * HOW DO WE KNOW WHEN WE ARE DONE?
 * When the number of responses recvd matches the number of requests made?
 *   This would imply that the subsidiary pages (linked-to pages) need to make requests
 * before the response from the parent page is sent back to the requestor.  Out of order.
 * A problem?  Possibly for the later data processing.
 *   The more general solution is to use a synch.WaitGroup.  It is a counter meant for this
 * purpose. Even so, we do not guarantee in-order results, since the linked-to page could have
 * been seen earlier, anyway.
 */
/* TODO:  Better throttling — done.
 * Todo:  Use command-line arguments, at least for the test: Not supported by the testing package, though.
 * Todo:  Put all this in a separate function: shouldWeContinue() bool
 */
var urlFindRe *regexp.Regexp // Find linked-to urls in body of page
var fileRe *regexp.Regexp
var urlRejectRe *regexp.Regexp // Further examined links to ensure reasonable.

// Prevent duplicate fetching, by keeping urls in a map.  And counters.
// Mentally keep track of the shared data by putting it in a struct.  The struct provides
// no safety or synchronization by itself, it's just a reminder.
type synchStuff struct {
	sync.RWMutex      // "Embedding" type without variable name makes using it more convenient.
	beenThereDoneThat map[string]bool
	urlsFetched       int
	dupsStopped       int
	queueCnt          int
	rejectCnt         int
}

var synched synchStuff

var maxUrlsFetched int = 544 // Over-ridden by loading in the test data file.
var maxGoRoutines int = 5    // Over-ridden ...

var goingCount int = 0
var startTime time.Time
var just1Domain bool = false // Don't go beyond the initial domain.
// Only makes sense when just one url in the starting array.

const reqChanCapacity = 60000

// This enables the program to wait until all the data is processed, and then exit.
var waitGroup sync.WaitGroup

// Includes the answer to the Get of the url, the url itself, error, elapsed time.
type ResponseFromWeb struct {
	Url          string         // Original url
	HttpResponse *http.Response // Response from http.Get()
	Err          error          // Error from Get()
	ElapsedTime  time.Duration  // Time duration of Get()
}

var respChan chan *ResponseFromWeb // result of Get() sent back along this channel to 'caller'.

// For a channel of url requests.  At one time I thought each request would be more than a string.
type RequestUrl struct {
	Url string
}

var reqChan chan *RequestUrl // Channel of requests from main program to this package.

var fileClient *http.Client // For handling file:// urls.  Good for testing, mainly.

var routineStatus []rune // A status letter for the running-state of each goroutine.  For diagnosis.

func init() {
	// Bug: What if there is a newline in the "wrong" place?
	urlFindRe = regexp.MustCompile(`href="((https?|file)://[^">#\?]+)`) // Note the back-quotes
	fileRe = regexp.MustCompile(`(file)://`)
	urlRejectRe = regexp.MustCompile(`\.(css|ico|js|py|pdf|png|mp3|mp4|jpg|jpeg|swf|exe|dll|so|lib)\/?$`)

	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	fileClient = &http.Client{Transport: t}
}

// Main External entry point for package creep.  Call only once at a time, but you can give
// it an array of urls to process.
func CreepWebSites(urls []string, maxPermittedUrls int, maxGoRo int, justOneDomain bool) <-chan *ResponseFromWeb {

	startTime = time.Now()
	maxUrlsFetched = maxPermittedUrls
	just1Domain = justOneDomain
	synched.beenThereDoneThat = make(map[string]bool, 20+maxUrlsFetched)

	maxGoRoutines = maxGoRo
	fmt.Printf(" Creeping: %2d maxUrlsFetched, %2d maxGoRoutines, %5d reqChanCapacity, %s justOneDomain.\n",
		maxUrlsFetched, maxGoRoutines, reqChanCapacity, boolTF(just1Domain))
	respChan = make(chan *ResponseFromWeb, 100)       // buffered
	reqChan = make(chan *RequestUrl, reqChanCapacity) // Big buffer.  Enough?

	// TODO:   WHAT TO DO WHEN CHANNEL IS FULL:  STASH ON THE DISK??  Probably just block.  But
	// blocking means that the goroutine isn't running to finish jobs that would cause it to
	// unblock.  Fatal embrace?

	synched.urlsFetched = 0
	synched.dupsStopped = 0 // Don't need synchronization yet.  Not until goroutines.

	routineStatus = make([]rune, 1+maxGoRoutines) // Allocate the strings for diagnostic array.

	if 1 > len(urls) {
		log.Fatal("No urls to process")
	}

	for w := 0; w < maxGoRoutines; w++ { // Start the worker goroutines.
		go getUrl(reqChan, respChan, 1+w)
	}

	if just1Domain { // I think this is the only reasonable case that can actually terminate.
		if 1 < len(urls) {
			log.Fatal("For just-one-domain, must have just one starting url")
		}
		setCurrentDomain(urls[0])
		enQueue(urls[0], reqChan, 0)
	} else {
		for _, thisurl := range urls { // For each url in the initial test list.
			enQueue(thisurl, reqChan, 0)
		}
	}
	fmt.Printf(" Waiting:\n")
	go waitUntilDone(reqChan)
	routineStatus[0] = 'x' // main entry pt returning caller: goroutines are started.
	return respChan
}

// Simply wait until all done.  I suppose we could do this with a special channel.
// This is actually easier.  MAYBE NOT.
func waitUntilDone(reqChan chan *RequestUrl) {
	time.Sleep(10 * time.Second)
	waitGroup.Wait()
	sendAllDone(9999999) // If we ever decide to use 9,999,999 goroutines then this fails...
	fmt.Printf("Waited: req:resp %3d:%3d. %5d urls fetched, %5d in map, %5d dupes stopped, %5d rejected, at %v\n",
		len(reqChan), len(respChan), synched.urlsFetched, mapLength(), synched.dupsStopped, synched.rejectCnt,
		time.Since(startTime))
	log.Println("go Status: ", string(routineStatus))
	close(reqChan)
	time.Sleep(time.Second)
	close(respChan)
}

// Display a bool as a single letter, T or F.
func boolTF(george bool) string {
	if george {
		return "T"
	} else {
		return "F"
	}
}

/* getUrl is the worker goroutine for getting an url and processing it:
 * The number of such work routines is fixed at program startup time.
 */
func getUrl(reqChan chan *RequestUrl, respChan chan *ResponseFromWeb, routineNumber int) {
	routineStatus[routineNumber] = '0' // virgin.  No activity yet.
	for {
		routineStatus[routineNumber] = 'W' // Blocked waiting on request channel.
		theReq, ok := <-reqChan
		if !ok {
			break // request channel is closed.  Who closes this?  We do.  So won't happen here?
		}
		routineStatus[routineNumber] = 'G' // Going
		thisUrl := theReq.Url

		synched.Lock()
		killSelf := synched.urlsFetched > maxUrlsFetched
		synched.Unlock()

		if killSelf {
			routineStatus[routineNumber] = 'K' // First killSelf — too many urls.
			fmt.Printf("\t ->>1 Too many urls fetched %4d after %v\n", synched.urlsFetched, time.Since(startTime))
			sendAllDone(routineNumber) // End-of-stream back to caller.
			return
		}

		goingCount++
		reallyGetUrl(thisUrl, reqChan, respChan, routineNumber) // Do the bulk of the work.
	}
	routineStatus[routineNumber] = 'X' // This goroutine is eXiting.
}

/* reallyGetUrl() is a not-strictly-necessary subroutine of getUrl that does most of the work.
 *  1. See if we have got this url already.
 *  2. See if we have exceeded number of url fetches permitted.
 *  3. Use the right protocol to get the file:// or the http:// url.
 *  4. Send the response back on the response channel.
 *  5. Search the response body for links that we ought to follow.
 */
func reallyGetUrl(thisUrl string, reqChan chan *RequestUrl, respChan chan *ResponseFromWeb, routineNumber int) {
	// in lieu of canonicalizing, remove trailing slash from thisUrl:
	if 12 > len(thisUrl) {
		fmt.Println("URL too short ", len(thisUrl), thisUrl)
		return
	}
	if thisUrl[len(thisUrl)-1] == '/' {
		thisUrl = thisUrl[0 : len(thisUrl)-1]
		//fmt.Printf("=Deslashed= '%s'\n", thisUrl  )
	}

	synched.Lock() // Write-lock shared data.
	if synched.beenThereDoneThat[thisUrl] {
		// been there, done that. Check the url, possibly for the 2nd time, if was Q'ed from a link.
		synched.dupsStopped++
		synched.Unlock()
		routineStatus[routineNumber] = 'D' // url rejected cause it's Duplicate.  Been there done that.
		return
	}

	synched.beenThereDoneThat[thisUrl] = true // Requires a write lock.
	syUrlFetched := synched.urlsFetched

	if syUrlFetched > maxUrlsFetched {
		synched.Unlock()
		routineStatus[routineNumber] = 'T' // Another case of too many urls.
		fmt.Printf("\t ->>2 Too many urls fetched %4d after %v\n", syUrlFetched, time.Since(startTime))
		sendAllDone(routineNumber) // End-of-stream back to caller.
		return
	}
	synched.urlsFetched++ // Requires a write lock. It counts attempts that succeed or fail.
	synched.Unlock()      // DON'T want to defer this, want to release it asap.

	var client *http.Client

	if nil != fileRe.FindStringIndex(thisUrl) {
		client = fileClient
		//fmt.Printf("file: protocol for %s\n", thisUrl)
	} else {
		client = http.DefaultClient
		//fmt.Printf("http: protocol for %s\n", thisUrl)
	}

	waitGroup.Add(1)
	/*  Keep track of number of urls we Get().  So we know when we are all done.
	 *  This might not yield correct behavior if we terminate early?  Such as
	 *  when exceeding number of urls allowed or any other resource constraint.
	 */
	routineStatus[routineNumber] = 'F' // Begun Fetching from the web.
	fmt.Printf("=Fetching=  '%s'\n", thisUrl)
	start := time.Now()
	getResponse, getErr := client.Get(thisUrl) // This can take a long time.  Hundreds of milliseconds.
	getElapsed := time.Since(start)

	// Check again to see if we are over the limit in the total number of urls fetched:
	synched.Lock()
	syUrlFetch := synched.urlsFetched // It's been a long time since we last checked this
	synched.Unlock()

	killit := syUrlFetch > maxUrlsFetched

	if killit {
		routineStatus[routineNumber] = 'L' // Second attempt to killSelf because too many urls.
		fmt.Printf("=ENDING: after %5d: Too many urls fetched\n", syUrlFetch)
		sendAllDone(routineNumber) // End-of-stream back to caller.
	}

	if (nil == getResponse) && (nil == getErr) {
		e := errors.New("-->> NIL RESP AND NIL ERR RETURN! for '" + thisUrl + "' <<--")
		log.Fatalf("Error %v\n", e) // Practically never happens.
		return
	}

	if nil == getErr { // Send back the success, this page before the linked-to pages.
		routineStatus[routineNumber] = 'R' // Sending response back to caller.
		sendResponse(thisUrl, getResponse, getErr, getElapsed)
		routineStatus[routineNumber] = 'S' // Sent response back to caller.

		fmt.Printf("\n%35s: %s, stat %3d, len %3d, Elapsed %s\n", thisUrl, getResponse.Status,
			getResponse.StatusCode, getResponse.ContentLength, getElapsed.String())
		searchBodyForLinks(getResponse, reqChan, routineNumber) // Look for the later, linked-to pages.
	} else { // Send back the error
		routineStatus[routineNumber] = 'E' // Send error response back to caller.
		sendResponse(thisUrl, getResponse, getErr, getElapsed)
	}

	waitGroup.Done() // This url is all done.  Do this AFTER queueing up found links.
	// Otherwise the number of outstanding links could become zero, which
	// would make the waitGroup think we are all done with work.
	// Bug: There is no way to guarantee against this waitGroup error, is there?

	if killit {
		routineStatus[routineNumber] = 'N' // DONE because of too many urls.
		sendAllDone(routineNumber)         // End-of-stream back to caller.
	}

	if (nil != getResponse) && (nil != getResponse.Body) {
		getResponse.Body.Close()
	}
}

// Send a special terminating message back along the response channel; After all urls are done.
func sendAllDone(routineNumber int) {
	fmt.Printf("SEND ALL DONE by %2d at delta %v\n", routineNumber, time.Since(startTime))
	fmt.Println("go Status: ", string(routineStatus))
	respChan <- &ResponseFromWeb{"DONE", nil, nil, time.Duration(0)} // back to caller.
}

// Send the response to the 'caller' along the response channel.
func sendResponse(thisUrl string, getResponse *http.Response, getErr error, elapse time.Duration) {
	respChan <- &ResponseFromWeb{thisUrl, getResponse, getErr, elapse}
	//Bug: But what about rejected Urls???  How do we .Done() them?   We never .Add() them
}

// Use regex to search Body of the web page for links to other web pages, and follow them in turn.
func searchBodyForLinks(httpResp *http.Response, reqChan chan *RequestUrl, rn int) {
	body, getErr := ioutil.ReadAll(httpResp.Body)
	if getErr != nil {
		fmt.Println(getErr) // Really ought to recover from this.
		return
	}
	routineStatus[rn] = 'A' // Searching body for Anchor links to enQueue
	links := urlFindRe.FindAllStringSubmatch(string(body), -1)
	//fmt.Printf("%3d links found\n", len(links))
	for _, alink := range links {
		enQueue(alink[1], reqChan, rn)
		// alink[1] is just the url.  the [0] includes the 'href=' part of the regular expression.
	}
}

// Try to enQueue a url, see and count whether it is accepted or rejected.
// If okay, send it along the request channel.
func enQueue(thisUrl string, reqChan chan *RequestUrl, rn int) {
	var rejectThisUrl bool
	rejectThisUrl = nil != urlRejectRe.FindStringIndex(thisUrl) // Don't follow *.css etc
	if rejectThisUrl {
		//fmt.Printf("REJECT1: '%s'\n", thisUrl) // Not a web page.
	} else if just1Domain {
		rejectThisUrl = !hasSameDomain(thisUrl)
		// if rejectThisUrl {
		// 	fmt.Printf("REJECT2: '%s'\n", thisUrl) // link goes offsite
		// }
	}

	synched.Lock() // Write lock shared data.  There's just too much of this.

	if rejectThisUrl {
		synched.rejectCnt++
	} else {
		synched.queueCnt++
	}

	// Heuristic check this for the first time.  Don't queue iff already done it.
	if synched.beenThereDoneThat[thisUrl] {
		// been there, done that.
		synched.dupsStopped++
		synched.Unlock()
		// routineStatus[routineNumber] = 'd' // url rejected cause it's Duplicate.  Been there done that.
		return
	}
	synched.Unlock() // Write unlock ...

	if rejectThisUrl {
		return
	}
	routineStatus[rn] = 'C'              // enQueue blocked waiting on request channel.
	reqChan <- &RequestUrl{Url: thisUrl} // Here's the beef; Put it on the channel.
	routineStatus[rn] = 'e'              // enQueue is returning.
}

// get length of the list-of-urls map, with a synchronized read.
func mapLength() int {
	synched.Lock()
	ml := len(synched.beenThereDoneThat)
	synched.Unlock()
	return ml
}
