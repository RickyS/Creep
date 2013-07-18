//  coding: utf-8
// :samedomain.go by Ricky Seltzer rickyseltzer@gmail.com.  Version 1.0 on 2013-06-27

package creep

import (
	//"fmt"
	"github.com/joeguo/tldextract"
	"log"
)

var cache string
var extract *tldextract.TLDExtract

func init() {
	cache = "/tmp/tld.cache"
	extract = tldextract.New(cache, false)
}

var theFirstUrl string // set this in case we ever need it.
var currentDomain string

func setCurrentDomain(firstUrl string) {
	theFirstUrl = firstUrl
	result := extract.Extract(firstUrl)
	currentDomain = result.Root + "." + result.Tld
	log.Printf("\tCurrent Domain: %+v => (%s)\n-----\n", result, currentDomain)
}

func hasSameDomain(newUrl string) bool {
	result := extract.Extract(newUrl)
	newDom := result.Root + "." + result.Tld
	return newDom == currentDomain
}
