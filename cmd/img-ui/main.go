// Command img-ui is the web interface to the toolkit.
//
// It serves a page from a local server and drives the same code the commands
// do, through internal/ops. Nothing here knows how to quantise or resize; it
// knows how to ask. That is deliberate — the interface cannot drift from the
// command line if there is only one implementation between them.
//
// The choice of a local server over a desktop window is also deliberate. The
// page talks plain HTTP and JSON, so the same interface runs in a browser tab
// today and inside a native window later without a line of it changing.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

type server struct {
	mu    sync.RWMutex
	root  string // no path outside this is ever touched
	token string
	fs    http.Handler
}

// Root is where the tools are allowed to look. It starts at whatever the
// command line named and can be moved from the page, because picking a folder
// is the first thing anyone wants to do and a person at the keyboard is
// entitled to reach their own disk. What the root defends against is a stray
// request, not its owner — the real boundary is the loopback address and the
// token, and both hold wherever the root points.
func (s *server) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root
}

func (s *server) setRoot(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	s.mu.Lock()
	s.root = abs
	s.mu.Unlock()
	return nil
}

func main() {
	var port int
	var open, fresh bool
	flag.IntVar(&port, "port", 0, "port to listen on, 0 picks a free one")
	flag.BoolVar(&open, "open", true, "open the page in the default browser")
	flag.BoolVar(&fresh, "new-token", false, "mint a new token, invalidating any open tab")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-ui — the web interface to the toolkit\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-ui [flags] [directory]\n\n")
		fmt.Fprintf(os.Stderr, "The directory is the root: nothing outside it can be read or\n")
		fmt.Fprintf(os.Stderr, "written. It defaults to the current one.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %s is not a directory\n", abs)
		os.Exit(2)
	}

	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	srv := &server{root: abs, token: loadToken(fresh), fs: http.FileServer(http.FS(assets))}

	// Loopback only. This process can read and write files, so it has no
	// business listening anywhere a machine on the network could reach it.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://%s/#t=%s", ln.Addr().String(), srv.token)
	fmt.Printf("pixelita — serving %s\n%s\n", abs, url)
	if open {
		go func() {
			time.Sleep(200 * time.Millisecond)
			openBrowser(url)
		}()
	}

	http.Serve(ln, srv)
}

// loadToken is what stops another page in the same browser from driving this
// server. A local port is not a secret — any site the user visits can try to
// reach it — but the token in the address is one, and a cross-origin script
// cannot read it.
//
// It is kept in a file rather than minted per run. A token that changes on
// every restart quietly breaks every tab that is already open: the page keeps
// its old one, every call comes back 403, and nothing on screen says why. The
// file is readable only by its owner, which is the same protection the terminal
// output had.
func loadToken(fresh bool) string {
	path := ""
	if dir, err := os.UserConfigDir(); err == nil {
		dir = filepath.Join(dir, "pixelita")
		if os.MkdirAll(dir, 0o700) == nil {
			path = filepath.Join(dir, "token")
		}
	}
	if path != "" && !fresh {
		if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
			return string(b)
		}
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token := hex.EncodeToString(b)
	if path != "" {
		os.WriteFile(path, []byte(token), 0o600)
	}
	return token
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		// Files embedded in a binary carry no modification time, so a browser
		// caches them by its own guess and a rebuilt binary can serve a stale
		// page. There is no network between us worth caching for.
		w.Header().Set("Cache-Control", "no-cache")
		s.fs.ServeHTTP(w, r)
		return
	}

	// A cross-site request cannot read the token, but it can still be sent
	// blind, so a foreign origin is refused outright.
	//
	// The test is "is this loopback" rather than "does this equal our host".
	// Comparing to the host looks stricter and is merely brittle: 127.0.0.1 and
	// localhost are the same server under two names, a proxy rewrites the host
	// header, and the failure only shows on POST — because a browser omits
	// Origin on same-origin GETs. What that strictness would buy is nothing:
	// the token is the boundary, and a page on another loopback port can no
	// more read it than a page on the open internet.
	if origin := r.Header.Get("Origin"); origin != "" && !isLoopback(origin) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	// An <img src> cannot carry a header, so the picture endpoints take the
	// token in the query instead. It is the same secret either way; a URL is
	// the weaker place to keep one, which is why it is accepted and never
	// required.
	if r.Header.Get("X-Pixelita-Token") != s.token && r.URL.Query().Get("t") != s.token {
		http.Error(w, "bad or missing token", http.StatusForbidden)
		return
	}
	s.routes(w, r)
}

func isLoopback(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

// resolve turns a path from the page into a real one, and refuses anything that
// tries to leave the root.
func (s *server) resolve(rel string) (string, error) {
	root := s.Root()
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	full := filepath.Join(root, filepath.FromSlash(rel))

	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		real = full // it may not exist yet; the prefix check below still applies
	}
	if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return "", errors.New("path is outside the root")
	}
	return full, nil
}

// rel is the inverse: what the page should see.
func (s *server) rel(full string) string {
	r, err := filepath.Rel(s.Root(), full)
	if err != nil {
		return filepath.ToSlash(full)
	}
	return filepath.ToSlash(r)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "could not open a browser; use the address above")
	}
}
