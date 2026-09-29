package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
)

var networkCreateRe = regexp.MustCompile(`^(/v[0-9.]+)?/networks/create$`)

func main() {
	target, _ := url.Parse("http://127.0.0.1:2375")
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director

	proxy.Director = func(req *http.Request) {
		orig(req)
		if req.Method == "POST" && networkCreateRe.MatchString(req.URL.Path) {
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				return
			}
			var data map[string]interface{}
			if json.Unmarshal(body, &data) == nil {
				opts, ok := data["Options"].(map[string]interface{})
				if !ok || opts == nil {
					opts = map[string]interface{}{}
				}
				opts["com.docker.network.driver.mtu"] = "1400"
				data["Options"] = opts
				body, _ = json.Marshal(data)
			}
			req.Body = ioutil.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
			req.Header.Set("Content-Length", strconv.Itoa(len(body)))
		}
	}

	log.Println("mtu-proxy listening on :2376, forwarding to :2375")
	log.Fatal(http.ListenAndServe(":2376", proxy))
}
