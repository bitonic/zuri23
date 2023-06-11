const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
var tokenId = -1;
var playerId = -1;
var puzzleId = -1;
var started = false;

var tokens = [
    { txt: "...", X: 50 + Math.random() * 100, Y: 50 + Math.random() * 500 },
];

var moving = true;

canvas.addEventListener("mousemove", e => {
    if (!moving || tokenId < 0) { return; }
    const rect = canvas.getBoundingClientRect();
    if (started) {
        tokens[tokenId].X = (e.pageX - rect.left) / rect.width;
        tokens[tokenId].Y = (e.pageY - rect.top) / rect.height;
    }
    render();
    send();
});

const params = new URLSearchParams(window.location.search);
const debug = params.has('debug')
if (debug) {
    canvas.addEventListener('click', () => {
        moving = !moving;
    })
}

function render() {
    const ctx = canvas.getContext("2d");
    ctx.font = "30px monospace";
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    ctx.fillStyle = "rgba(200, 200, 0, 0.3)";
    ctx.fillRect(0, 0.4 * canvas.height, canvas.width, 0.2 * canvas.height);

    ctx.strokeStyle = "rgb(160, 160, 255)";
    for (let i  = 0; i < 10; i++) {
        ctx.beginPath();
        ctx.moveTo(0,            canvas.height * i / 10);
        ctx.lineTo(canvas.width, canvas.height * i / 10);
        ctx.stroke();
    }
    ctx.beginPath();
    ctx.moveTo(canvas.width * 0.1, 0);
    ctx.lineTo(canvas.width * 0.1, canvas.height);
    ctx.stroke();

    for (let i in tokens) {
        const token = tokens[i];
        if (i == tokenId) {
            ctx.fillStyle = "rgb(130, 110, 0)";
        } else {
            ctx.fillStyle = "rgb(0, 100, 100)";
        }
        ctx.fillText(token.Token, token.X * canvas.width, token.Y * canvas.height);
    }
}

const wsUrl = new URL('/ws', window.location.href);
wsUrl.protocol = wsUrl.protocol.replace('http', 'ws');
wsUrl.href
const socket = new WebSocket(wsUrl.href);

socket.addEventListener("message", (event) => {
        const obj = JSON.parse(event.data);
        tokenId = obj.TokenID
        playerId = obj.PlayerID
        started = obj.Started;
        let i;
        for (let i = 0; i < obj.Tokens.length; i++) {
            const tok = tokens[i];
            tokens[i] = obj.Tokens[i];
            if (i == tokenId && tok) {
                tokens[i].X = tok.X;
                tokens[i].Y = tok.Y;
            }
        }
        for (let i = 0; i < tokens.length - obj.Tokens.length; i++) {
            console.log('popping')
            tokens.pop()
        }

        if (tokenId >= 0) {
            document.getElementById('youreplaying').innerHTML = "You're playing! " + (debug ? (" " + playerId) : "");
        } else {
            document.getElementById('youreplaying').innerHTML = (debug ? (" " + playerId) : "");
        }

        puzzleId = obj.PuzzleID;
        document.getElementById("goal").innerHTML = obj.PuzzleGoal
        if (obj.LevelClear) {
                document.getElementById("ghci").classList.add("won");
        } else {
                document.getElementById("ghci").classList.remove("won");
        }
        document.getElementById("playercount").innerHTML = obj.Players + "/" + tokens.length;
        document.getElementById("lobbycount").innerHTML = obj.Lobby;
        //console.log(obj);
        document.getElementById("ghci").innerHTML = obj.GHCIOutput;
        render();
        send();
})

// A client update waiting to be sent to the server
var dataToSend = null;

function send() {
    dataToSend = JSON.stringify({
        "PuzzleID": puzzleId,
        "TokenID": tokenId,
        "PlayerID": playerId,
        "X": tokens[tokenId].X,
        "Y": tokens[tokenId].Y
    });
}

function sendData() {
    if (dataToSend !== null) {
        socket.send(dataToSend);
    }
    setTimeout(sendData, 25);
}
setTimeout(sendData, 25);

function resizeCanvas() {
    // Enforce 2 aspect ratio
    canvas.height = canvas.clientHeight
    canvas.width = canvas.clientHeight*2
    render();
}

window.addEventListener('resize', resizeCanvas);
resizeCanvas();
