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
		puzzle{
			goal: "[0,1,2,3,4]",
			tokens: []puzzleToken{
				puzzleToken{Token: "take",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "5",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "iterate",  tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "(+1)",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "0",        tokenLoc: tokenLoc{50, 50}},
			},
		},
		puzzle{
			goal: "32",
			tokens: []puzzleToken{
				puzzleToken{Token: "iterate",  tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "(",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "join",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "(+)",      tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: ")",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "1",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "!!",       tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "5",        tokenLoc: tokenLoc{50, 50}},
		},
		puzzle{
			goal: "e",
			tokens: []puzzleToken{
				puzzleToken{Token: "succ",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "sum",      tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "scanl1",   tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "(/)",      tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "[1..100]", tokenLoc: tokenLoc{50, 50}},
			},
		},
		puzzle{
			goal: "8",
			tokens: []puzzleToken{
				puzzleToken{Token: "let",      tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "a",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "+",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "b",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "=",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "a",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "*",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "b",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "in",       tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "2",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "+",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "2",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "+",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "2",        tokenLoc: tokenLoc{50, 50}},
			},
		},
		puzzle{
			goal: "\"fin\"",
			tokens: []puzzleToken{
				puzzleToken{Token: "take",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "3",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "drop",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "2",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "show",     tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "$",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "1",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "/",        tokenLoc: tokenLoc{50, 50}},
				puzzleToken{Token: "0",        tokenLoc: tokenLoc{50, 50}},
			},
		}

	evalReqs  = make(chan string, 16)
	evalResps = make(chan string)

	updates = make(chan clientUpdate, 32)

	subChan = make(chan subReq)
)

type puzzleState struct {
	currentPuzzle int
	ghciOut       string
	tokens        []puzzleToken
	goal          string
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
			r := postResponse{
				GHCIOutput: s.ghciOut,
				PuzzleGoal: s.goal,
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

		case u := <-updates:
			if u.puzzleID == s.currentPuzzle && u.clientID >= 0 && u.clientID < len(s.tokens) {
				log.Printf("update[%d]: %+v", u.clientID, u.loc)
				s.tokens[u.clientID].tokenLoc = u.loc
			}
			updateClients()

			tokens := arrange(s.tokens)
			expr := strings.Join(
				Map(tokens, puzzleToken.token),
				" ")
			evalReqs <- expr

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

func evaluate(input string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "/usr/bin/env", "ghci", "-e", input).CombinedOutput()
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
			}
			last = current
			current = ""
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
				puzzleID: 0,
				clientID: postReq.ClientID,
				loc:      tokenLoc{postReq.X, postReq.Y},
			}
		}

	}()

	subChan <- subReq{responses, stop}
	for r := range responses {
		if err := websocket.Message.Send(ws, string(r)); err != nil {
			close(stop)
		}
	}
	ws.Close()
	wg.Wait()

}
