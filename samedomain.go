//  coding: utf-8
// :samedomain.go by Ricky Seltzer rickyseltzer@gmail.com.  Version 1.0 on 2013-06-27

package creep

import (
	"github.com/joeguo/tldextract"
	"log"
	"sync"
)

var cache string
var extract *tldextract.TLDExtract

var theFirstUrl string // set this in case we ever need it.
var currentDomain string

var domainChan chan string // A channel of copied domain strings.

type synchDomainType struct {
	sync.RWMutex
	domains map[string]bool // map of domains, to avoid double queueing.  Actually duplicates
	// the contents of the domain channel, I should think.
	domainCnt int
}

var synchedDomainMap synchDomainType

func init() {
	cache = "/tmp/tld.cache"
	extract = tldextract.New(cache, false)

	// map of domains to avoid duplicate search-and-fetch of domains.
	synchedDomainMap.domains = make(map[string]bool, 5500)
	synchedDomainMap.domainCnt = 0
	domainChan = make(chan string, 5500)
}

func enQueueNewDomain(thisDomain string, rn int) {

	synchedDomainMap.Lock()
	if !synchedDomainMap.domains[thisDomain] {
		synchedDomainMap.domains[thisDomain] = true
		synchedDomainMap.domainCnt++
		synchedDomainMap.Unlock() // unlock before sending to channel.  Which could block.
		log.Printf("\tenQueue new domain '%s'. len %3d\n", thisDomain, len(domainChan))
		routineStatus[rn] = 'q' // Queue domain
		domainChan <- thisDomain
	} else {
		synchedDomainMap.Unlock()
	}
}

func setCurrentDomain(firstUrl string) {
	theFirstUrl = firstUrl
	currentDomain = getDomain(firstUrl)
	log.Printf("\tCurrent Domain: %s\n-----\n", currentDomain)
}

func hasSameDomain(newUrl string) bool {
	return getDomain(newUrl) == currentDomain
}

func getDomain(someUrl string) string {
	x := extract.Extract(someUrl)
	return x.Root + "." + x.Tld
}
