package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
)

func main() {
	http.HandleFunc("/post", handlePost)
	http.Handle("/", http.FileServer(http.Dir("../frontend")))
	http.ListenAndServe("0.0.0.0:8001", http.DefaultServeMux)
}

type postRequest struct {
	Txt string  `json:"txt"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
}

type postResponse struct {
	GHCIOutput string
}

func handlePost(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	reqBody, _ := ioutil.ReadAll(req.Body)

	var postReq postRequest
	err := json.Unmarshal(reqBody, &postReq)
	if err != nil {
		panic(err)
	}

	fmt.Printf(">>> %+v\n", postReq)

	out := runGhci("aoeu")
	resp, _ := json.Marshal(postResponse{out})
	w.Write(resp)
}

func runGhci(input string) string {
	out, err := exec.Command("/usr/bin/ghci", "-e", input).CombinedOutput()
	if err != nil {
		return string(out) + "(" + err.Error() + ")"
	}
	return string(out)
}
