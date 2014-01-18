Package to crawl the web. Used by main program Crawl.
=======================================================

To install:  
       $ go get github.com/RickyS/Crawl  
       $ go get github.com/RickyS/Creep  

You'll neeed both packages, the depend on each other.  The main program is crawl. 
The working package is creep.  Note the capital letters on the names to 'go get'.

The easiest introduction might be to run  
      go test  
This runs for 9 seconds on my system.  

Package creep implements a web crawler.  It reads web pages and follows links to the rest of
the web, recursively, ad infinitum, within the limits provided.  We use the term creep to avoid name clashes
with other software called 'walk' and 'crawl'.  I'm thinking of changing it to 'stroll'.

The goroutines in crawl.go listens on a request channel and then scans the web page specified in the message from the 
request channel.  Each link-to-another-web-page found is then enqueued onto the request channel.  Eventually, this or another goroutine will read that request and process it.

The code in samedomain.go uses the package "github.com/joeguo/tldextract" to get
the database to help figure out whether two different URLs belong to the same domain.
It turns out that this is not as simple as it might seem.

In order to prevent infinite regress, the program limits operation to the list of domains
in the json file.

There are parameters in the json file that adjust the limitations.  TBD.
