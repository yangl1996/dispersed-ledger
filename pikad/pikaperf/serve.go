package main

import (
	"bytes"
	"net/http"
	"os"
	"time"
)

func serveBwPlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-bw.svg")
}

func serveMemPlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-mem.svg")
}

func serveQueuePlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-q.svg")
}

func serveProgressPlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-proto.svg")
}

func serveConfirmationPlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-cfm.svg")
}

func serveMessagePlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-msg.svg")
}

func serveBlockPlot(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "./plot-blk.svg")
}

func serveLiveJs(w http.ResponseWriter, req *http.Request) {
	http.ServeContent(w, req, "live.js", time.Time{}, livejs)
}

func serveHtml(w http.ResponseWriter, req *http.Request) {
	// we are going to change the modified time of this html to match the svg
	// so that we can fool the js to reload the page when the svg changes
	// this is because the js will not be triggered by changing the svg itself
	file, err := os.Stat("./plot-bw.svg")
	if err != nil {
		dpLogger.Fatalf("Error stating the plot: %v\n", err)
	}
	http.ServeContent(w, req, "index.html", file.ModTime(), htmltext)
}

func serve() {
	http.HandleFunc("/bwplot.svg", serveBwPlot)
	http.HandleFunc("/memplot.svg", serveMemPlot)
	http.HandleFunc("/queueplot.svg", serveQueuePlot)
	http.HandleFunc("/protoplot.svg", serveProgressPlot)
	http.HandleFunc("/cfmplot.svg", serveConfirmationPlot)
	http.HandleFunc("/blkplot.svg", serveBlockPlot)
	http.HandleFunc("/msgplot.svg", serveMessagePlot)
	http.HandleFunc("/live.js", serveLiveJs)
	http.HandleFunc("/view", serveHtml)
	go http.ListenAndServe("localhost:8080", nil)
}

var htmltext = bytes.NewReader([]byte(`
<!doctype html>
<html lang=en>
<head>
<meta charset=utf-8>
<title>Pika Live</title>
<script type="text/javascript" src="/live.js"></script>
</head>
<body>
<img src="/bwplot.svg">
<img src="/protoplot.svg">
<img src="/queueplot.svg">
<img src="/cfmplot.svg">
<img src="/blkplot.svg">
<img src="/msgplot.svg">
<img src="/memplot.svg">
</body>
</html>
`))
