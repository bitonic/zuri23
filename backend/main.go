package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
	"bytes"

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

type puzzle struct {
	goal   string
	tokens []puzzleToken
}

type clientUpdate struct {
	puzzleID int
	clientID int
	loc      tokenLoc
	response chan postResponse
}

type subReq struct {
	responses chan []byte
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
				{Token: "let", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "a", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "+", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "b", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "=", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "a", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "*", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "b", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "in", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "2", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "+", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "2", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "+", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "2", tokenLoc: tokenLoc{0.5, 0.5}},
			},
		},
		{
			goal: "\"fin\"",
			tokens: []puzzleToken{
				{Token: "take", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "3", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "$", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "drop", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "2", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "$", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "show", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "$", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "1", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "/", tokenLoc: tokenLoc{0.5, 0.5}},
				{Token: "0", tokenLoc: tokenLoc{0.5, 0.5}},
			},
		},
	}

	evalReqs  = make(chan string, 16)
	evalResps = make(chan string)

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

	for {
		select {
		case <-updateTrigger:
			tokens := arrange(s.tokens)
			expr := strings.Join(
				Map(tokens, puzzleToken.token),
				" ")

			s.levelClear = slices.Equal(tokens, s.tokens)
			evalReqs <- expr

			r := postResponse{
				GHCIOutput: s.ghciOut,
				PuzzleGoal: s.goal,
				PuzzleID:   s.currentPuzzle,
				Tokens:     slices.Clone(s.tokens),
			}
			bs, _ := json.Marshal(r)
			for id, sub := range subs {
				select {
				case sub.responses <- bs:
				case <-sub.stop:
					close(sub.responses)
					delete(subs, id)
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
					s.currentPuzzle--
					s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
					s.goal = puzzles[s.currentPuzzle].goal
					log.Printf("moved to puzzle %d", s.currentPuzzle)
				}
			case "next":
				if s.currentPuzzle+1 < len(puzzles) {
					s.levelClear = false
					s.currentPuzzle++
					s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
					s.goal = puzzles[s.currentPuzzle].goal
					log.Printf("moved to puzzle %d", s.currentPuzzle)
				}
			default:
				log.Printf("unknown control command %q", c)
			}
			updateClients()

		case u := <-updates:
			if s.levelStarted && !s.levelClear && u.puzzleID == s.currentPuzzle && u.clientID >= 0 && u.clientID < len(s.tokens) {
				log.Printf("update[%d]: %+v", u.clientID, u.loc)
				s.tokens[u.clientID].tokenLoc = u.loc

			}
			updateClients()

		case s := <-subChan:
			subs[subId] = s
			subId += 1
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
	ClientID int
	PuzzleID int
	X        float64
	Y        float64
}

type postResponse struct {
	GHCIOutput string
	PuzzleID   int
	PuzzleGoal string
	Tokens     []puzzleToken
}

func ws(ws *websocket.Conn) {
	responses := make(chan []byte, 5)
	stop := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			var bs []byte
			err := websocket.Message.Receive(ws, &bs)
			if err != nil {
				return
			}

			var postReq postRequest
			err = json.Unmarshal(bs, &postReq)
			if err != nil {
				continue
			}
			updates <- clientUpdate{
				puzzleID: postReq.PuzzleID,
				clientID: postReq.ClientID,
				loc:      tokenLoc{postReq.X, postReq.Y},
			}
		}

	}()

	subChan <- subReq{responses, stop}
	for r := range responses {
		if err := websocket.Message.Send(ws, string(r)); err != nil {
			close(stop)
			break
		}
	}
	ws.Close()
	wg.Wait()

}
