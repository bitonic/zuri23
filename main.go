package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/net/websocket"
)

type tokenLoc struct {
	X, Y float64
}

type puzzleToken struct {
	tokenLoc
	Token string
}

type puzzle struct {
	goal        string
	startTokens []puzzleToken
}

type tokenUpdate struct {
	PuzzleID int
	TokenID  int
	X        float64
	Y        float64
}

var (
	puzzles = []puzzle{
		{
			goal: "[0,1,2,3,4]",
			startTokens: []puzzleToken{
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
			startTokens: []puzzleToken{
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
			startTokens: []puzzleToken{
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
			startTokens: []puzzleToken{
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
			startTokens: []puzzleToken{
				{Token: "take", tokenLoc: tokenLoc{0.76, 0.65}},
				{Token: "3", tokenLoc: tokenLoc{0.65, 0.31}},
				{Token: "$", tokenLoc: tokenLoc{0.13, 0.82}},
				{Token: "drop", tokenLoc: tokenLoc{0.87, 0.79}},
				{Token: "2", tokenLoc: tokenLoc{0.58, 0.31}},
				{Token: "$", tokenLoc: tokenLoc{0.19, 0.76}},
				{Token: "show", tokenLoc: tokenLoc{0.69, 0.83}},
				{Token: "$", tokenLoc: tokenLoc{0.2, 0.88}},
				{Token: "1", tokenLoc: tokenLoc{0.5, 0.31}},
				{Token: "/", tokenLoc: tokenLoc{0.8, 0.9}},
				{Token: "0", tokenLoc: tokenLoc{0.41, 0.31}},
			},
		},
	}

	tokenUpdates = make(chan tokenUpdate, 128)
)

type playerId int64

// update we send to the player
type playerResp struct {
	PuzzleGoal string
	GHCIOutput string
	Tokens     []puzzleToken
	PuzzleID   int
	Players    int
	Lobby      int
	TokenID    int
	Started    bool
	LevelClear bool
}

type player struct {
	updates chan playerResp
}

type state struct {
	mu            sync.RWMutex
	playerIdCount int64
	// index into `puzzles`
	currentPuzzle int
	// all players
	players map[playerId]player
	// currently playing. if -1, not playing. if positive, the token.
	active map[playerId]int
	// as currently arranged by the clients
	tokens []puzzleToken
	// current output
	ghciOut string
	// we're playing
	started bool
}

func shuffledKeys[K comparable, V any](m map[K]V) []K {
	keys := maps.Keys(m)
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	return keys
}

// modifications not protected by lock
// --------------------------------------------------------------------

func (s *state) reshuffleActive() {
	for pid := range s.players {
		s.active[pid] = -1
	}
	pids := shuffledKeys(s.players)
	for i, pid := range pids {
		if i < len(s.tokens) {
			s.active[pid] = i
		} else {
			s.active[pid] = -1
		}
	}
}

// stops the game, resets the players
func (s *state) stop() {
	s.started = false
	s.reshuffleActive()
}

// starts the game, fills the active players
func (s *state) start() {
	s.started = true
	s.reshuffleActive()
}

func (s *state) afterLevelChange() {
	s.started = false
	s.tokens = slices.Clone(puzzles[s.currentPuzzle].startTokens)
	s.ghciOut = "<n/a>"
	s.reshuffleActive()
}

func (s *state) next() {
	s.currentPuzzle++
	if s.currentPuzzle > len(puzzles) {
		s.currentPuzzle--
		return
	}
	s.afterLevelChange()
}

func (s *state) prev() {
	s.currentPuzzle--
	if s.currentPuzzle < 0 {
		s.currentPuzzle--
		return
	}
	s.afterLevelChange()
}

func (s *state) addPlayer() (playerId, *player) {
	s.playerIdCount++
	pid := playerId(s.playerIdCount)
	ch := make(chan playerResp, 5)
	p := player{updates: ch}
	s.players[pid] = p

	occupied := make([]bool, len(s.tokens))
	for _, tok := range s.active {
		if tok >= 0 {
			occupied[tok] = true
		}
	}
	firstFreeToken := -1
	for tok, occ := range occupied {
		if !occ {
			firstFreeToken = tok
			break
		}
	}
	s.active[pid] = firstFreeToken

	log.Printf("added player %v with token %v", pid, s.active[pid])

	return pid, &p
}

func (s *state) removePlayer(pid playerId) {
	delete(s.players, pid)
	freeTok := s.active[pid]
	// replace with somebody else
	if freeTok >= 0 {
		pids := shuffledKeys(s.players)
		for _, pid := range pids {
			if s.active[pid] < 0 {
				s.active[pid] = freeTok
			}
		}
	}
}

// end of lock-unprotected stuff
// --------------------------------------------------------------------

func (s *state) control(w http.ResponseWriter, req *http.Request) {
	log.Printf("control enter")
	s.mu.Lock()
	defer s.mu.Unlock()
	log.Printf("control locked")
	c := req.URL.RawQuery
	switch c {
	case "start":
		s.start()
	case "stop":
		s.stop()
	case "prev":
		s.prev()
	case "next":
		s.next()
	default:
		log.Printf("unknown control command %q", c)
	}
	log.Printf("control done")
}

func tokenStr(tokens []puzzleToken) string {
	return strings.Join(Map(tokens, func(t *puzzleToken) string { return t.Token }), " ")
}

func (s *state) sendUpdatesOnce() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	puzzle := &puzzles[s.currentPuzzle]
	update := playerResp{
		PuzzleGoal: puzzle.goal,
		Tokens:     slices.Clone(s.tokens),
		GHCIOutput: s.ghciOut,
		Started:    s.started,
		PuzzleID:   s.currentPuzzle,
	}
	sendUpdate := func(pid playerId, p *player) {
		thisUpdate := update
		thisUpdate.TokenID = s.active[pid]
		// log.Printf("sending update %+v", thisUpdate)
		select {
		case p.updates <- thisUpdate:
		default:
		}
	}
	for pid, p := range s.players {
		sendUpdate(pid, &p)
	}
}

func (s *state) sendUpdates() {
	for {
		// log.Printf("sending updates")
		s.sendUpdatesOnce()
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *state) processTokenUpdate(update *tokenUpdate) {
	// TODO make rlock
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentPuzzle != update.PuzzleID {
		log.Printf("bad puzzle id, %v vs %v", s.currentPuzzle, update.PuzzleID)
		return
	}

	token := s.tokens[update.TokenID]
	log.Printf("processing update for token %v (%s)", update.TokenID, token.Token)

	// log.Printf("tokens before %+v", s.tokens)
	s.tokens[update.TokenID].X = update.X
	s.tokens[update.TokenID].Y = update.Y
	// log.Printf("tokens before %+v", s.tokens)

}

func (s *state) processTokenUpdates() {
	for {
		update := <-tokenUpdates
		s.processTokenUpdate(&update)
	}
}

func (s *state) ws(ws *websocket.Conn) {
	s.mu.Lock()
	pid, p := s.addPlayer()
	s.mu.Unlock()

	errs := make(chan error, 2)

	go func() {
		for {
			var bs []byte
			err := websocket.Message.Receive(ws, &bs)
			if err != nil {
				errs <- err
				return
			}

			var req tokenUpdate
			err = json.Unmarshal(bs, &req)
			if err != nil {
				log.Printf("could not decode: %+v\n", err)
				continue
			}

			select {
			case tokenUpdates <- req:
			default:
				log.Printf("dropping token update\n")
			}
		}
	}()

	go func() {
		for r := range p.updates {
			bs, _ := json.Marshal(r)
			if err := websocket.Message.Send(ws, string(bs)); err != nil {
				errs <- err
				break
			}
		}
		errs <- nil
	}()

	// Wait for either fail to receive or send
	<-errs
	s.mu.Lock()
	s.removePlayer(pid)
	s.mu.Unlock()
	ws.Close()
	<-errs

}

func evaluate(input string) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmdCtx := exec.CommandContext(ctx, "/usr/bin/env", "runhaskell", "-XExtendedDefaultRules")
	cmdCtx.Stdin = bytes.NewReader([]byte("import Control.Monad\nsolution = " + input + "\nmain = print solution"))
	out, err := cmdCtx.CombinedOutput()
	if err != nil {
		return string(out) + "(" + err.Error() + ")"
	}
	return string(out)
}

func arrange(tokens []puzzleToken) []puzzleToken {
	filteredTokens := []puzzleToken{}
	for i := range tokens {
		if math.Abs(tokens[i].Y-0.5) < 0.1 {
			filteredTokens = append(filteredTokens, tokens[i])
		}
	}
	slices.SortFunc(filteredTokens, func(a, b puzzleToken) bool {
		return a.X < b.X
	})
	return filteredTokens
}

func Map[A, B any](xs []A, f func(*A) B) []B {
	out := make([]B, len(xs))
	for i := range xs {
		out[i] = f(&xs[i])
	}
	return out
}

// runs ghci once in a while
func (s *state) evaluator() {
	var last string
	for {
		s.mu.RLock()
		currentCode := tokenStr(arrange(s.tokens))
		s.mu.RUnlock()
		if currentCode != "" && currentCode != last {
			// log.Printf("evaluating %q", currentCode)
			result := evaluate(currentCode)
			// log.Printf("done evaluating %q", currentCode)
			s.mu.Lock()
			s.ghciOut = "λ> " + currentCode + "\n" + result
			s.mu.Unlock()
			last = currentCode
			currentCode = ""
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func main() {
	s := &state{
		currentPuzzle: 0,
		players:       make(map[playerId]player),
		active:        make(map[playerId]int),
	}
	s.afterLevelChange()

	go s.processTokenUpdates()
	go s.evaluator()
	go s.sendUpdates()

	http.HandleFunc("/control", func(w http.ResponseWriter, req *http.Request) { s.control(w, req) })
	http.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) { s.ws(ws) }))
	http.Handle("/", http.FileServer(http.Dir("frontend")))
	http.ListenAndServe("0.0.0.0:8001", http.DefaultServeMux)
}
