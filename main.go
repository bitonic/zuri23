package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/net/websocket"
)

func main() {
	s := &puzzleState{
		currentPuzzle: 0,
		ghciOut:       "<n/a>",
		tokens:        slices.Clone(puzzles[0].tokens),
		goal:          puzzles[0].goal,
	}
	go s.run()
	go evaluator()

	http.HandleFunc("/control", handleControl)

	http.Handle("/ws", websocket.Handler(ws))
	http.Handle("/", http.FileServer(http.Dir("frontend")))
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

type puzzle struct {
	goal   string
	tokens []puzzleToken
}

type clientUpdate struct {
	puzzleID int
	tokenID  int
	loc      tokenLoc
	response chan postResponse
}

type subReq struct {
	responses chan postResponse
	stop      chan struct{}
}

var (
	puzzles = []puzzle{
		{
			goal: "[0,1,2,3,4]",
			tokens: []puzzleToken{
				{Token: "take", tokenLoc: tokenLoc{0.76, 0.58}},
				{Token: "5", tokenLoc: tokenLoc{0.17, 0.56}},
				{Token: "$", tokenLoc: tokenLoc{0.36, 0.7}},
				{Token: "iterate", tokenLoc: tokenLoc{0.62, 0.21}},
				{Token: "(+1)", tokenLoc: tokenLoc{0.28, 0.24}},
				{Token: "0", tokenLoc: tokenLoc{0.5, 0.5}},
			},
		},
		{
			goal: "32",
			tokens: []puzzleToken{
				{Token: "iterate", tokenLoc: tokenLoc{0.7, 0.25}},
				{Token: "(", tokenLoc: tokenLoc{0.71, 0.44}},
				{Token: "join", tokenLoc: tokenLoc{0.27, 0.29}},
				{Token: "(+)", tokenLoc: tokenLoc{0.59, 0.62}},
				{Token: ")", tokenLoc: tokenLoc{0.5, 0.18}},
				{Token: "1", tokenLoc: tokenLoc{0.14, 0.59}},
				{Token: "!!", tokenLoc: tokenLoc{0.44, 0.46}},
				{Token: "5", tokenLoc: tokenLoc{0.14, 0.25}},
			},
		},
		{
			goal: "e",
			tokens: []puzzleToken{
				{Token: "succ", tokenLoc: tokenLoc{0.14, 0.77}},
				{Token: "$", tokenLoc: tokenLoc{0.45, 0.49}},
				{Token: "sum", tokenLoc: tokenLoc{0.79, 0.18}},
				{Token: "$", tokenLoc: tokenLoc{0.52, 0.48}},
				{Token: "scanl1", tokenLoc: tokenLoc{0.76, 0.79}},
				{Token: "(/)", tokenLoc: tokenLoc{0.45, 0.85}},
				{Token: "[1..100]", tokenLoc: tokenLoc{0.15, 0.25}},
			},
		},
		{
			goal: "8",
			tokens: []puzzleToken{
				{Token: "let", tokenLoc: tokenLoc{0.5824421466621129, 0.9569263934812886}},
				{Token: "a", tokenLoc: tokenLoc{0.6834105749801536, 0.6861474266283613}},
				{Token: "+", tokenLoc: tokenLoc{0.6352210978283614, 0.8146526990331403}},
				{Token: "b", tokenLoc: tokenLoc{0.7958526883343352, 0.9523369194668322}},
				{Token: "=", tokenLoc: tokenLoc{0.7384842631536302, 0.8123579620259122}},
				{Token: "a", tokenLoc: tokenLoc{0.7063579450524355, 0.9454527084451476}},
				{Token: "*", tokenLoc: tokenLoc{0.39083160655855836, 0.37635793065255463}},
				{Token: "b", tokenLoc: tokenLoc{0.6868526804909958, 0.8169474360403686}},
				{Token: "in", tokenLoc: tokenLoc{0.5939158316982538, 0.6907369006428177}},
				{Token: "2 + 2", tokenLoc: tokenLoc{0.24511580659956783, 0.36717898262364185}},
				{Token: "+", tokenLoc: tokenLoc{0.7648737387367546, 0.7045053226861868}},
				{Token: "2", tokenLoc: tokenLoc{0.4585263482717902, 0.36717898262364185}},
			},
		},
		{
			goal: "\"fin\"",
			tokens: []puzzleToken{
				{Token: "take", tokenLoc: tokenLoc{0.76, 0.65}},
				{Token: "3", tokenLoc: tokenLoc{0.65, 0.31}},
				{Token: "$", tokenLoc: tokenLoc{0.13, 0.82}},
				{Token: "drop", tokenLoc: tokenLoc{0.87, 0.79}},
				{Token: "2", tokenLoc: tokenLoc{0.58, 0.31}},
				{Token: "$", tokenLoc: tokenLoc{0.19, 0.76}},
				{Token: "show", tokenLoc: tokenLoc{0.69, 0.83}},
				{Token: "$", tokenLoc: tokenLoc{0.2, 0.88}},
				{Token: "1", tokenLoc: tokenLoc{0.5, 0.31}},
				{Token: "/", tokenLoc: tokenLoc{0.46, 0.64}},
				{Token: "0", tokenLoc: tokenLoc{0.41, 0.31}},
			},
		},
	}

	evalReqs  = make(chan string, 512)
	evalResps = make(chan string, 16)

	control = make(chan string)

	updates = make(chan clientUpdate, 32)

	subChan = make(chan subReq)
)

type puzzleState struct {
	currentPuzzle int
	ghciOut       string
	tokens        []puzzleToken
	goal          string
	levelClear    bool
	levelStarted  bool
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

func shuffledKeys[K comparable, V any](m map[K]V) []K {
	keys := maps.Keys(m)
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	return keys
}

func (s *puzzleState) run() {
	//newRoundCountdown := 50

	subId := int64(0)
	subs := map[int64]subReq{}

	updateTrigger := make(chan struct{}, 1)

	updateClients := func() {
		select {
		case updateTrigger <- struct{}{}:
		default:
		}
	}

	assignments := map[int64]int{} // Player to tokenID assignments
	unassigned := map[int]bool{}   // Unassigned tokens

	reassign := func() {
		defer log.Printf("reassign: assignments=%v, unassigned=%v", assignments, unassigned)

		available := map[int64]bool{} // The still available players
		for id := range subs {
			if _, ok := assignments[id]; !ok {
				available[id] = true
			}
		}
		if len(available) == 0 {
			return
		}
		availableShuffled := shuffledKeys(available)
		for _, tokenId := range shuffledKeys(unassigned) {
			if len(availableShuffled) == 0 {
				return
			}
			id := availableShuffled[0]
			availableShuffled = availableShuffled[1:]
			assignments[id] = tokenId
			unassigned[tokenId] = false
		}
	}

	for {
		select {
		case <-updateTrigger:
			tokens := arrange(s.tokens)
			expr := strings.Join(
				Map(tokens, puzzleToken.token),
				" ")

			s.levelClear = slices.Equal(tokens, s.tokens)
			if s.levelClear {
				log.Printf("level clear!")
			}

			select {
			case evalReqs <- expr:
			default:
			}

			r := postResponse{
				GHCIOutput: s.ghciOut,
				PuzzleGoal: s.goal,
				PuzzleID:   s.currentPuzzle,
				Tokens:     slices.Clone(s.tokens),
			}
			for id, sub := range subs {
				if tokenID, ok := assignments[id]; ok {
					r.TokenID = tokenID
				} else {
					r.TokenID = -1
				}
				select {
				case sub.responses <- r:
				case <-sub.stop:
					close(sub.responses)
					delete(subs, id)
					log.Printf("[%d] disconnected, %d/%d players", id, len(subs), len(s.tokens))
					if r.TokenID >= 0 {
						delete(assignments, id)
						unassigned[r.TokenID] = true
						reassign()
					}
				default:
				}
			}

		case c := <-control:
			switch c {
			case "start":
				s.levelStarted = true
				log.Printf("started")
			case "prev":
				if s.currentPuzzle-1 >= 0 {
					s.levelClear = false
					s.levelStarted = false
					s.currentPuzzle--
					s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
					s.goal = puzzles[s.currentPuzzle].goal
					log.Printf("moved to puzzle %d", s.currentPuzzle)
				}
			case "next":
				if s.currentPuzzle+1 < len(puzzles) {
					s.levelClear = false
					s.levelStarted = false
					s.currentPuzzle++
					s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
					s.goal = puzzles[s.currentPuzzle].goal
					log.Printf("moved to puzzle %d", s.currentPuzzle)
				}
			default:
				log.Printf("unknown control command %q", c)
			}

			// Redo assignments on each control message.
			assignments = map[int64]int{}
			unassigned = map[int]bool{}
			for i := range s.tokens {
				unassigned[i] = true
			}
			reassign()

			updateClients()

		case u := <-updates:
			if s.levelStarted && !s.levelClear && u.puzzleID == s.currentPuzzle && u.tokenID >= 0 && u.tokenID < len(s.tokens) {
				//log.Printf("update[%d]: %+v", u.tokenID, u.loc)
				s.tokens[u.tokenID].tokenLoc = u.loc

			}

			updateClients()

		case subReq := <-subChan:
			subs[subId] = subReq
			subId += 1
			log.Printf("%d/%d players", len(subs), len(s.tokens))
			if s.levelStarted {
				// If we've started reassign on new players
				// to "reassign" disconnected ones.
				reassign()
			}

			updateClients()

		case s.ghciOut = <-evalResps:
			updateClients()

		}
	}
}

func (s *puzzleState) next() bool {
	s.currentPuzzle += 1
	if s.currentPuzzle >= len(puzzles) {
		return false
	}

	s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
	return true
}

func handleControl(w http.ResponseWriter, req *http.Request) {
	control <- req.URL.RawQuery
}

func evaluate(input string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmdCtx := exec.CommandContext(ctx, "/usr/bin/env", "runhaskell")
	cmdCtx.Stdin = bytes.NewReader([]byte("import Control.Monad\nsolution = " + input + "\nmain = print solution"))
	out, err := cmdCtx.CombinedOutput()
	if err != nil {
		return string(out) + "(" + err.Error() + ")"
	}
	return string(out)
}

func evaluator() {
	ticks := time.NewTicker(100 * time.Millisecond)
	var (
		current string
		last    string
	)
	for {
		select {
		case new := <-evalReqs:
			current = new
		case <-ticks.C:
			if current != "" && current != last {
				log.Printf("evaluating %q", current)
				result := evaluate(current)
				evalResps <- "λ> " + current + "\n" + result
				last = current
				current = ""
			}
		}
	}
}

type postRequest struct {
	PuzzleID int
	X        float64
	Y        float64
}

type postResponse struct {
	GHCIOutput string
	PuzzleID   int
	PuzzleGoal string
	TokenID    int
	Tokens     []puzzleToken
}

func ws(ws *websocket.Conn) {
	tokenID := int(-1)

	responses := make(chan postResponse, 5)
	errs := make(chan error, 2)
	stop := make(chan struct{})

	go func() {
		for {
			var bs []byte
			err := websocket.Message.Receive(ws, &bs)
			if err != nil {
				errs <- err
				return
			}

			var postReq postRequest
			err = json.Unmarshal(bs, &postReq)
			if err != nil {
				continue
			}
			updates <- clientUpdate{
				puzzleID: postReq.PuzzleID,
				tokenID:  tokenID,
				loc:      tokenLoc{postReq.X, postReq.Y},
			}
		}
	}()

	go func() {
		subChan <- subReq{responses, stop}
		for r := range responses {
			tokenID = r.TokenID // yeah it's racy.
			bs, _ := json.Marshal(r)
			if err := websocket.Message.Send(ws, string(bs)); err != nil {
				errs <- err
				break
			}
		}
		// Drain.
		for range responses {
		}
		errs <- nil
	}()

	// Wait for either receive or send to fail.
	<-errs
	close(stop)
	ws.Close()
	<-errs
}
