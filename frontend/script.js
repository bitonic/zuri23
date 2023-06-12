const canvas = document.getElementById("notebook");
canvas.style.cursor = 'none';
var tokenId = -1;
var puzzleId = -1;

var tokens = [
    { txt: "...", X: 50 + Math.random() * 100, Y: 50 + Math.random() * 500 },
];

var moving = true;

canvas.addEventListener("mousemove", e => {
    if (!moving || tokenId < 0) { return; }
    const rect = canvas.getBoundingClientRect();
    tokens[tokenId].X = (e.pageX - rect.left) / rect.width;
    tokens[tokenId].Y = (e.pageY - rect.top) / rect.height;
    render();
    send();
});

canvas.addEventListener('click', () => {
    moving = !moving;
})

function render() {
    const ctx = canvas.getContext("2d");
    ctx.font = "30px monospace";
    ctx.clearRect(0, 0, canvas.width, canvas.height);
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
	console.log("message:", event.data);
        const obj = JSON.parse(event.data);
        tokenId = obj.TokenID
        for (let i in obj.Tokens) {
            if (i != tokenId) {
                tokens[i] = obj.Tokens[i];
            }
        }

        if (obj.PuzzleID != puzzleId) {
	        // New puzzle!
	        puzzleId = obj.PuzzleID;
	        tokens = obj.Tokens;
	        document.getElementById("goal").innerHTML = obj.PuzzleGoal

        }
        //console.log(obj);
        document.getElementById("ghci").innerHTML = obj.GHCIOutput;
        render();
})

// A client update waiting to be sent to the server
var dataToSend = null;

function send() {
    const timerExists = dataToSend !== null;
    dataToSend = JSON.stringify({
        "PuzzleID": puzzleId,
        "X": tokens[tokenId].X,
        "Y": tokens[tokenId].Y
    });
    if (!timerExists) {
        setTimeout(() => {
            if (dataToSend !== null) {
                socket.send(dataToSend);
                dataToSend = null;
            }
        }, 25);
    }
}

function resizeCanvas() {
    // Enforce 2 aspect ratio
    canvas.height = canvas.clientHeight
    canvas.width = canvas.clientHeight*2
    render();
}

window.addEventListener('resize', resizeCanvas);
resizeCanvas();
