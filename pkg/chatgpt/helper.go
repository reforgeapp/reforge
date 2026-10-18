package chatgpt

import (
	"encoding/json"
	"net/url"
	"strings"
)

func LocalHelper(authorization, callback string) (string, error) {
	location, err := url.Parse(authorization)
	if err != nil || location.Scheme != "https" || location.Host != "auth.openai.com" || location.User != nil || location.Path != "/api/accounts/authorize" || location.Fragment != "" || location.Query().Get("redirect_uri") != LocalRedirect || location.Query().Get("state") == "" {
		return "", ErrAuthorization
	}
	back, err := url.Parse(callback)
	if err != nil || back.Scheme != "https" || back.Host == "" || back.User != nil || back.Path != "/auth/openai/callback" || back.RawQuery != "" || back.Fragment != "" {
		return "", ErrAuthorization
	}
	auth, _ := json.Marshal(authorization)
	returnURL, _ := json.Marshal(callback)
	return strings.NewReplacer("{{authorization}}", string(auth), "{{callback}}", string(returnURL)).Replace(localHelper), nil
}

const localHelper = `import hmac
import http.server
import time
import urllib.parse
import webbrowser

authorization = {{authorization}}
callback = {{callback}}
expected_state = urllib.parse.parse_qs(urllib.parse.urlsplit(authorization).query)["state"][0]
complete = False

class Callback(http.server.BaseHTTPRequestHandler):
    def setup(self):
        super().setup()
        self.connection.settimeout(5)

    def log_message(self, *args):
        pass

    def do_GET(self):
        global complete
        parsed = urllib.parse.urlsplit(self.path)
        query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
        state = query.get("state", [])
        keys = ("state", "code", "client_id", "error")
        valid = self.headers.get("Host") == "127.0.0.1:1456" and parsed.path == "/auth/callback" and len(self.path) <= 16384 and len(state) == 1 and state[0].isascii() and hmac.compare_digest(state[0], expected_state) and all(len(query.get(key, [])) <= 1 for key in keys)
        self.send_response(303 if valid else 400)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("Content-Security-Policy", "default-src 'none'")
        if valid:
            returned = {key: query[key][0] for key in keys if key in query}
            self.send_header("Location", callback + "?" + urllib.parse.urlencode(returned))
            complete = True
        self.send_header("Content-Length", "0")
        self.end_headers()

def main():
    with http.server.HTTPServer(("127.0.0.1", 1456), Callback) as server:
        server.timeout = 1
        if not webbrowser.open(authorization):
            print("Could not open browser. Run this helper on your desktop computer.")
            return
        print("Complete ChatGPT sign-in in your browser. This helper exits automatically.")
        deadline = time.monotonic() + 600
        while not complete and time.monotonic() < deadline:
            server.handle_request()
        if not complete:
            print("Sign-in expired. Download a new helper from Reforge.")

if __name__ == "__main__":
    try:
        main()
    except OSError:
        print("Could not start local callback. Close another Reforge sign-in helper and try again.")
    except KeyboardInterrupt:
        pass
`
