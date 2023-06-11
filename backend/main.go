package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

func main() {
	s := &puzzleState{
		currentPuzzle: 0,
		ghciOut:       "<n/a>",
		tokens:        slices.Clone(puzzles[0]),
	}
	go s.run()

	http.HandleFunc("/post", handlePost)
	http.Handle("/", http.FileServer(http.Dir("../frontend")))
	http.ListenAndServe("0.0.0.0:8001", http.DefaultServeMux)
}

type tokenLoc struct {
	X, Y float64
}

type puzzleToken struct {
	tokenLoc
	Token     string
	imageFile string
}

func (t puzzleToken) token() string {
	return t.Token
}

func (t puzzleToken) Loc() tokenLoc {
	return t.tokenLoc
}

type puzzle []puzzleToken

type clientUpdate struct {
	puzzleID int
	clientID int
	loc      tokenLoc
	response chan postResponse
}

func mk(tokens ...string) puzzle {
	return Map(tokens, func(t string) puzzleToken {
		return puzzleToken{
			tokenLoc: tokenLoc{50, 50},
			Token:    t,
		}
	})
}

var (
	puzzles = []puzzle{
		mk("foo", "bar", "baz"),
	}

	updates = make(chan clientUpdate, 32)
)

type puzzleState struct {
	currentPuzzle int
	ghciOut       string
	tokens        []puzzleToken
}

func Map[A, B any](xs []A, f func(A) B) []B {
	out := make([]B, len(xs))
	for i := range xs {
		out[i] = f(xs[i])
	}
	return out
}

func arrange(tokens []puzzleToken) []puzzleToken {
	tokens = slices.Clone(tokens)
	slices.SortFunc(tokens, func(a, b puzzleToken) bool {
		return a.X < b.X
	})
	return tokens
}

func (s *puzzleState) run() {
	ticks := time.NewTicker(100 * time.Millisecond)
	//newRoundCountdown := 50

	for {
		select {
		case u := <-updates:
			if u.puzzleID == s.currentPuzzle && u.clientID >= 0 && u.clientID < len(s.tokens) {
				log.Printf("update[%d]: %+v", u.clientID, u.loc)
				s.tokens[u.clientID].tokenLoc = u.loc
			}
			u.response <- postResponse{
				GHCIOutput: s.ghciOut,
				Tokens:     slices.Clone(s.tokens),
			}
			close(u.response)

		case <-ticks.C:
			tokens := arrange(s.tokens)
			expr := strings.Join(
				Map(tokens, puzzleToken.token),
				" ")
			s.ghciOut = evaluate(expr)
			//log.Printf("evaluate(%s): %s", expr, s.ghciOut)

			/*
				if slices.Equal(s.tokens, tokens) {
					newRoundCountdown--
					if newRoundCountdown == 0 {
						newRoundCountdown = 50
						s.next()
					}
				}*/

		}
	}
}

func (s *puzzleState) next() bool {
	s.currentPuzzle += 1
	if s.currentPuzzle >= len(puzzles) {
		return false
	}

	s.tokens = slices.Clone(puzzles[s.currentPuzzle])
	return true
}

type postRequest struct {
	ClientID int
	PuzzleID int
	X        float64
	Y        float64
}

type postResponse struct {
	GHCIOutput string
	PuzzleID   int
	Tokens     []puzzleToken
}

func handlePost(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	reqBody, _ := ioutil.ReadAll(req.Body)

	var postReq postRequest
	err := json.Unmarshal(reqBody, &postReq)
	if err != nil {
		panic(err)
	}
	log.Printf("request: %+v", postReq)

	resp := make(chan postResponse, 1)
	updates <- clientUpdate{
		puzzleID: 0,
		clientID: postReq.ClientID,
		loc:      tokenLoc{postReq.X, postReq.Y},
		response: resp,
	}
	respJson, _ := json.Marshal(<-resp)
	log.Printf("responding: %q", respJson)
	w.Write(respJson)
}

func evaluate(input string) string {
	out, err := exec.Command("/usr/bin/env", "ghci", "-e", input).CombinedOutput()
	if err != nil {
		return string(out) + "(" + err.Error() + ")"
	}
	return string(out)
}
