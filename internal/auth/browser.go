package auth

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the user's default browser.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default: // linux, bsd, ...
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// writeBrowserPage renders a minimal "you can close this tab" page in the
// browser after the OAuth redirect lands on our loopback server.
func writeBrowserPage(w http.ResponseWriter, ok bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Authentication complete"
	body := "You're signed in to the ClickFunnels CLI. You can close this tab and return to your terminal."
	if !ok {
		title = "Authentication failed"
		body = "Something went wrong: " + detail + ". Return to your terminal and try again."
	}
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;background:#0b0b0f;color:#e6e6e6;display:flex;height:100vh;margin:0;align-items:center;justify-content:center}
.card{max-width:420px;padding:2rem;text-align:center}h1{font-size:1.3rem;margin:0 0 .5rem}p{color:#9a9aa6;line-height:1.5}</style></head>
<body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, title, body)
}
