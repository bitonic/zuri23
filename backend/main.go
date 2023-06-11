package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/net/websocket"
)

func main() {
	s := &puzzleState{
		currentPuzzle: 0,
		ghciOut:       "<n/a>",
		expr:          "",
		tokens:        slices.Clone(puzzles[0].tokens),
		goal:          puzzles[0].goal,
	}
	go s.run()

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

func mk(goal string, tokens ...string) puzzle {
    return puzzle {
        goal: goal,
	tokens: Map(tokens, func(t string) puzzleToken {
		return puzzleToken{
			tokenLoc: tokenLoc{50, 50},
			Token:    t,
		}
	}),
    }
}

type subReq struct {
	responses chan []byte
	stop      chan struct{}
}

var (
	puzzles = []puzzle{
		mk("[0,1,2,3,4]", "take", "5", "$", "iterate", "(+1)", "0"),
	}

	updates = make(chan clientUpdate, 32)

	subChan = make(chan subReq)
)

type puzzleState struct {
	currentPuzzle int
	ghciOut       string
	expr          string
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
	ticks := time.NewTicker(100 * time.Millisecond)
	//newRoundCountdown := 50

	subId := int64(0)
	subs := map[int64]subReq{}

	trigger := make(chan struct{}, 1)

	retrigger := func() {
		select {
		case trigger <- struct{}{}:
		default:
		}
	}

	for {
		select {
		case <-trigger:
			r := postResponse{
				GHCIOutput: s.expr + ": " + s.ghciOut,
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
			retrigger()

		case s := <-subChan:
			subs[subId] = s
			subId += 1
			retrigger()

		case <-ticks.C:
			tokens := arrange(s.tokens)
			expr := strings.Join(
				Map(tokens, puzzleToken.token),
				" ")
			s.expr = expr
			s.ghciOut = evaluate(expr)
			retrigger()
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

	s.tokens = slices.Clone(puzzles[s.currentPuzzle].tokens)
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
	PuzzleGoal string
	Tokens     []puzzleToken
}

func evaluate(input string) string {
	out, err := exec.Command("/usr/bin/env", "ghci", "-e", input).CombinedOutput()
	if err != nil {
		return string(out) + "(" + err.Error() + ")"
	}
	return string(out)
}

func ws(ws *websocket.Conn) {
	responses := make(chan []byte, 5)
	stop := make(chan struct{})

	go func() {
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
		//log.Printf("sending response: %q", r)
		if err := websocket.Message.Send(ws, string(r)); err != nil {
			close(stop)
		}
	}
	for range responses {
	}
	ws.Close()

}
