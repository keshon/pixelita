package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/jpegopt"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

func (s *server) routes(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/dirs":
		s.handleDirs(w, r)
	case "/api/browse":
		s.handleBrowse(w, r)
	case "/api/root":
		s.handleRoot(w, r)
	case "/api/quit":
		s.handleQuit(w, r)
	case "/api/scan":
		s.handleScan(w, r)
	case "/api/apply":
		s.handleApply(w, r)
	case "/api/thumb":
		s.handleThumb(w, r)
	case "/api/image":
		s.handleImage(w, r)
	case "/api/candidate":
		s.handleCandidate(w, r)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}

// --- the folder tree -------------------------------------------------------

type dirEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Images   int    `json:"images"`
	Children bool   `json:"children"`
}

func (s *server) handleDirs(w http.ResponseWriter, r *http.Request) {
	full, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err, http.StatusForbidden)
		return
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		fail(w, err, http.StatusNotFound)
		return
	}

	// An empty list has to marshal as [] rather than null: a consumer that has
	// to special-case "no subfolders" is a consumer we made work for nothing.
	dirs := []dirEntry{}
	images := 0
	for _, e := range entries {
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
				continue
			}
			child := filepath.Join(full, e.Name())
			n, sub := summarise(child)
			dirs = append(dirs, dirEntry{
				Name: e.Name(), Path: s.rel(child), Images: n, Children: sub,
			})
			continue
		}
		if imgio.HasExt(e.Name()) {
			images++
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })

	writeJSON(w, map[string]any{
		"path": s.rel(full), "root": filepath.Base(s.Root()), "rootPath": s.Root(),
		"dirs": dirs, "images": images,
	})
}

// handleBrowse walks the machine outside the root, so that a folder can be
// chosen in the first place. An empty path lists the volumes.
func (s *server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, map[string]any{"path": "", "up": "", "dirs": volumes()})
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		fail(w, err, http.StatusNotFound)
		return
	}

	dirs := []dirEntry{}
	images := 0
	for _, e := range entries {
		if e.IsDir() {
			if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
				continue
			}
			child := filepath.Join(abs, e.Name())
			n, sub := summarise(child)
			dirs = append(dirs, dirEntry{Name: e.Name(), Path: child, Images: n, Children: sub})
			continue
		}
		if imgio.HasExt(e.Name()) {
			images++
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })

	up := filepath.Dir(abs)
	if up == abs {
		up = "" // a volume root: the only way further out is the list of volumes
	}
	writeJSON(w, map[string]any{"path": abs, "up": up, "dirs": dirs, "images": images})
}

func volumes() []dirEntry {
	var out []dirEntry
	if runtime.GOOS == "windows" {
		for c := byte('A'); c <= 'Z'; c++ {
			p := string(c) + `:\`
			if _, err := os.Stat(p); err == nil {
				out = append(out, dirEntry{Name: string(c) + ":", Path: p, Children: true})
			}
		}
		return out
	}
	return []dirEntry{{Name: "/", Path: "/", Children: true}}
}

// handleQuit stops the server. A local application started from a terminal has
// to be closeable from its own window; otherwise the only way out is to go and
// find the terminal it came from.
func (s *server) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"stopped": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		os.Exit(0)
	}()
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	if err := s.setRoot(req.Path); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"root": filepath.Base(s.Root()), "rootPath": s.Root()})
}

// summarise counts the images directly inside a directory and says whether it
// has subdirectories, so the tree knows what is worth opening.
func summarise(dir string) (images int, children bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	for _, e := range entries {
		switch {
		case e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "node_modules":
			children = true
		case !e.IsDir() && imgio.HasExt(e.Name()):
			images++
		}
	}
	return images, children
}

// --- streaming a run -------------------------------------------------------

// stream sends one JSON object per line as the work happens.
//
// A folder of a few hundred photographs takes minutes to measure honestly, and
// a progress bar that only moves at the end is not a progress bar. The same
// shape would serve the command line if it ever grows a streaming mode.
type stream struct {
	w   http.ResponseWriter
	enc *json.Encoder
	mu  sync.Mutex
}

func newStream(w http.ResponseWriter) *stream {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	return &stream{w: w, enc: json.NewEncoder(w)}
}

func (s *stream) send(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enc.Encode(v)
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}

// each runs fn over the files, streaming every result, and stops early if the
// browser goes away.
func (s *server) each(r *http.Request, out *stream, files []string, fn func(string) []report.Item) *report.Report {
	rep := report.New("img-ui", "done", false)
	out.send(map[string]any{"type": "start", "files": len(files)})

	var mu sync.Mutex
	var done int
	workers := min(runtime.NumCPU(), max(len(files), 1))
	queue := make(chan int)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range queue {
				items := fn(files[idx])
				mu.Lock()
				done++
				for _, it := range items {
					it.Path = s.rel(it.Path)
					if it.Output != "" {
						it.Output = s.rel(it.Output)
					}
					rep.Add(it)
					out.send(map[string]any{"type": "item", "item": it, "done": done})
				}
				mu.Unlock()
			}
		}()
	}

	for i := range files {
		select {
		case <-r.Context().Done():
			close(queue)
			wg.Wait()
			return rep
		case queue <- i:
		}
	}
	close(queue)
	wg.Wait()
	return rep
}

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	full, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err, http.StatusForbidden)
		return
	}
	files, err := listImages(full, r.URL.Query().Get("recursive") == "1")
	if err != nil {
		fail(w, err, http.StatusNotFound)
		return
	}

	opt := ops.DefaultScan()
	opt.Quick = r.URL.Query().Get("quick") == "1"

	out := newStream(w)
	rep := s.each(r, out, files, func(p string) []report.Item {
		return []report.Item{ops.Scan(p, opt)}
	})
	ops.ScanSummary(rep, opt)
	rep.Finish()
	out.send(map[string]any{"type": "done", "summary": rep.Summary, "notes": rep.Notes})
}

type applyRequest struct {
	Op    string   `json:"op"`
	Paths []string `json:"paths"`

	Colors  int     `json:"colors"`
	Dither  float64 `json:"dither"`
	Effort  int     `json:"effort"`
	Quality int     `json:"quality"`
	Mode    string  `json:"mode"`
	Width   int     `json:"width"`
	MaxSide int     `json:"maxSide"`

	MinGain float64 `json:"minGain"`
	MinPSNR float64 `json:"minPSNR"`
	DryRun  bool    `json:"dryRun"`
	Replace bool    `json:"replace"`
}

func (s *server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}

	files := make([]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		full, err := s.resolve(p)
		if err != nil {
			fail(w, err, http.StatusForbidden)
			return
		}
		files = append(files, full)
	}

	run, err := s.operation(req)
	if err != nil {
		fail(w, err, http.StatusBadRequest)
		return
	}

	out := newStream(w)
	rep := s.each(r, out, files, run)
	rep.Finish()
	out.send(map[string]any{"type": "done", "summary": rep.Summary})
}

// operation picks what the run will do to each file. The options come from the
// page, but the defaults and the decisions are the same ones the commands use.
func (s *server) operation(req applyRequest) (func(string) []report.Item, error) {
	switch req.Op {
	case "quant":
		o := ops.DefaultQuant()
		o.DryRun, o.Replace = req.DryRun, req.Replace
		if req.Colors > 0 {
			o.Colors = req.Colors
		}
		if req.Effort > 0 {
			o.Effort = req.Effort
		}
		o.Dither = req.Dither
		o.MinGain, o.MinPSNR = req.MinGain, req.MinPSNR
		return func(p string) []report.Item { return []report.Item{ops.Quant(p, o)} }, nil

	case "webp":
		o := ops.DefaultWebP()
		o.DryRun = req.DryRun
		if req.Quality > 0 {
			o.Quality = req.Quality
		}
		if req.Mode != "" {
			o.Mode = req.Mode
		}
		o.MinGain = req.MinGain
		if _, err := o.EncoderOptions(); err != nil {
			return nil, err
		}
		return func(p string) []report.Item { return []report.Item{ops.WebP(p, o)} }, nil

	case "resize":
		o := ops.DefaultResize()
		o.DryRun, o.Replace = req.DryRun, req.Replace
		o.Width = req.Width
		o.MaxWidth = req.MaxSide
		return func(p string) []report.Item { return ops.Resize(p, o) }, nil

	case "jpeg":
		o := ops.DefaultJPEG()
		o.DryRun, o.Replace = req.DryRun, req.Replace
		o.MinGain = req.MinGain
		return func(p string) []report.Item { return []report.Item{ops.JPEG(p, o)} }, nil
	}
	return nil, fmt.Errorf("unknown operation %q", req.Op)
}

func listImages(dir string, recursive bool) ([]string, error) {
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return []string{dir}, nil
	}
	if recursive {
		var out []string
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
					if p != dir {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if imgio.HasExt(p) {
				out = append(out, p)
			}
			return nil
		})
		sort.Strings(out)
		return out, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && imgio.HasExt(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// --- pictures --------------------------------------------------------------

func (s *server) handleImage(w http.ResponseWriter, r *http.Request) {
	full, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err, http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, full)
}

// thumbCache keeps the small versions in memory. Scrolling a folder of three
// hundred photographs would otherwise decode each one again on every pass.
var thumbCache sync.Map // key -> []byte

func (s *server) handleThumb(w http.ResponseWriter, r *http.Request) {
	full, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		fail(w, err, http.StatusForbidden)
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if width <= 0 || width > 1024 {
		width = 160
	}

	info, err := os.Stat(full)
	if err != nil {
		fail(w, err, http.StatusNotFound)
		return
	}
	key := fmt.Sprintf("%s|%d|%d|%d", full, info.ModTime().UnixNano(), info.Size(), width)
	if cached, ok := thumbCache.Load(key); ok {
		servePNG(w, cached.([]byte))
		return
	}

	img, _, _, err := imgio.Load(full)
	if err != nil {
		fail(w, err, http.StatusUnsupportedMediaType)
		return
	}
	src := imgio.ToNRGBA(img)
	tw, th := resize.Fit(src.Rect.Dx(), src.Rect.Dy(), width, width, "inside")
	if tw > src.Rect.Dx() {
		tw, th = src.Rect.Dx(), src.Rect.Dy()
	}
	data, err := imgio.EncodePNG(resize.Resize(src, tw, th, resize.CatmullRom))
	if err != nil {
		fail(w, err, http.StatusInternalServerError)
		return
	}
	thumbCache.Store(key, data)
	servePNG(w, data)
}

func servePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// handleCandidate produces what an operation would write, without writing it.
// This is what makes the before-and-after panel honest: it shows the actual
// bytes that would land on disk, not a preview rendered some other way.
func (s *server) handleCandidate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	full, err := s.resolve(q.Get("path"))
	if err != nil {
		fail(w, err, http.StatusForbidden)
		return
	}

	switch q.Get("op") {
	case "quant":
		o := ops.DefaultQuant()
		if n, err := strconv.Atoi(q.Get("colors")); err == nil && n > 0 {
			o.Colors = n
		}
		if v, err := strconv.ParseFloat(q.Get("dither"), 64); err == nil {
			o.Dither = v
		}
		if n, err := strconv.Atoi(q.Get("effort")); err == nil && n > 0 {
			o.Effort = n
		}
		res, reason, err := ops.QuantBytes(full, o)
		if err != nil {
			http.Error(w, reason+": "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("X-Bytes-Before", strconv.FormatInt(res.BytesBefore, 10))
		w.Header().Set("X-Bytes-After", strconv.Itoa(len(res.Encoded)))
		w.Header().Set("X-Colors", strconv.Itoa(res.Colors))
		if !math.IsInf(res.PSNR, 1) {
			w.Header().Set("X-PSNR", fmt.Sprintf("%.2f", res.PSNR))
		}
		servePNG(w, res.Encoded)

	case "webp":
		o := ops.DefaultWebP()
		if n, err := strconv.Atoi(q.Get("quality")); err == nil && n > 0 {
			o.Quality = n
		}
		if m := q.Get("mode"); m != "" {
			o.Mode = m
		}
		data, before, reason, err := ops.WebPBytes(full, o)
		if err != nil {
			http.Error(w, reason+": "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("X-Bytes-Before", strconv.FormatInt(before, 10))
		w.Header().Set("X-Bytes-After", strconv.Itoa(len(data)))
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)

	case "jpeg":
		raw, err := os.ReadFile(full)
		if err != nil {
			fail(w, err, http.StatusNotFound)
			return
		}
		out, err := jpegopt.Optimise(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("X-Bytes-Before", strconv.Itoa(len(raw)))
		w.Header().Set("X-Bytes-After", strconv.Itoa(len(out)))
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(out)

	case "resize":
		o := ops.DefaultResize()
		width, _ := strconv.Atoi(q.Get("width"))
		if width <= 0 {
			http.Error(w, "width is required", http.StatusBadRequest)
			return
		}
		img, format, _, err := imgio.Load(full)
		if err != nil {
			fail(w, err, http.StatusUnsupportedMediaType)
			return
		}
		src := imgio.ToNRGBA(img)
		tw, th := resize.Fit(src.Rect.Dx(), src.Rect.Dy(), width, 0, "inside")
		data, _, err := ops.EncodeAs(resize.Resize(src, tw, th, o.Filter), format, o, nil)
		if err != nil {
			fail(w, err, http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Bytes-After", strconv.Itoa(len(data)))
		servePNG(w, data)

	default:
		http.Error(w, "unknown operation", http.StatusBadRequest)
	}
}
