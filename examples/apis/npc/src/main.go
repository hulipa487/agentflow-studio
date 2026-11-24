package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

const (
	defaultNPCURL = "https://flk.npc.gov.cn"
	maxNPCCount   = 50
)

//go:wasmimport af_host http_request
func hostHTTPRequest(reqPtr, reqLen uint32) uint64

//go:wasmimport af_host log
func hostLog(level uint32, msgPtr, msgLen uint32)

var pinned = map[uint32][]byte{}

//go:wasmexport alloc
func alloc(size uint32) uint32 {
	if size == 0 {
		size = 1
	}
	buf := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	pinned[ptr] = buf
	return ptr
}

//go:wasmexport dealloc
func dealloc(ptr, size uint32) {
	delete(pinned, ptr)
}

func readGuest(ptr, size uint32) []byte {
	if ptr == 0 || size == 0 {
		return nil
	}
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), size))
	return out
}

func packResp(v any) uint64 {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"ok":false,"error":"failed to marshal wasm response"}`)
	}
	p := alloc(uint32(len(b)))
	copy(pinned[p], b)
	return (uint64(p) << 32) | uint64(len(b))
}

func logMsg(level uint32, msg string) {
	if msg == "" {
		return
	}
	buf := []byte(msg)
	hostLog(level, uint32(uintptr(unsafe.Pointer(&buf[0]))), uint32(len(buf)))
	runtime.KeepAlive(buf)
}

type hostHTTPResponse struct {
	OK         bool              `json:"ok"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodyBase64 string            `json:"body_base64"`
	Truncated  bool              `json:"truncated"`
	Error      string            `json:"error"`
}

func doHostHTTP(req map[string]any) (*hostHTTPResponse, error) {
	hb, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal host http request: %w", err)
	}
	if len(hb) == 0 {
		hb = []byte("{}")
	}
	packed := hostHTTPRequest(uint32(uintptr(unsafe.Pointer(&hb[0]))), uint32(len(hb)))
	runtime.KeepAlive(hb)
	if packed == 0 {
		return nil, fmt.Errorf("host http call returned null")
	}
	hPtr := uint32(packed >> 32)
	hLen := uint32(packed & 0xffffffff)
	hBytes := readGuest(hPtr, hLen)
	dealloc(hPtr, hLen)

	var resp hostHTTPResponse
	if err := json.Unmarshal(hBytes, &resp); err != nil {
		return nil, fmt.Errorf("decode host http response: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "unknown host http error"
		}
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

//go:wasmexport af_invoke
func afInvoke(reqPtr, reqLen uint32) uint64 {
	raw := readGuest(reqPtr, reqLen)
	var req struct {
		Action string         `json:"action"`
		Args   map[string]any `json:"args"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return packResp(map[string]any{"ok": false, "error": err.Error()})
	}

	var (
		data any
		err  error
	)
	switch req.Action {
	case "search":
		data, err = handleSearch(req.Args)
	case "read":
		data, err = handleRead(req.Args)
	default:
		err = fmt.Errorf("legal npc: unknown action %q", req.Action)
	}
	if err != nil {
		return packResp(map[string]any{"ok": false, "error": err.Error()})
	}
	return packResp(map[string]any{"ok": true, "data": data})
}

var npcStatus = map[int]string{
	1: "已废止",
	2: "已修改",
	3: "有效",
	4: "尚未生效",
}

var npcEmRe = regexp.MustCompile(`<[^>]*>`)

type npcSearchRequest struct {
	SearchRange   int      `json:"searchRange"`
	Sxrq          []string `json:"sxrq"`
	Gbrq          []string `json:"gbrq"`
	SearchType    int      `json:"searchType"`
	Sxx           []string `json:"sxx"`
	GbrqYear      []string `json:"gbrqYear"`
	FlfgCodeID    []string `json:"flfgCodeId"`
	ZdjgCodeID    []string `json:"zdjgCodeId"`
	SearchContent string   `json:"searchContent"`
	XgzlSearch    bool     `json:"xgzlSearch"`
	OrderByParam  struct {
		Order string `json:"order"`
		Sort  string `json:"sort"`
	} `json:"orderByParam"`
	PageNum  int `json:"pageNum"`
	PageSize int `json:"pageSize"`
}

type npcSearchResponse struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Total int    `json:"total"`
	Rows  []struct {
		BBBS     string `json:"bbbs"`
		Title    string `json:"title"`
		Gbrq     string `json:"gbrq"`
		Sxrq     string `json:"sxrq"`
		Sxx      int    `json:"sxx"`
		ZdjgName string `json:"zdjgName"`
		Flxz     string `json:"flxz"`
	} `json:"rows"`
}

type npcHit struct {
	Title     string `json:"title"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	Court     string `json:"court"`
	Date      string `json:"date"`
	Type      string `json:"type,omitempty"`
	Effective string `json:"effective,omitempty"`
	Status    string `json:"status,omitempty"`
}

func handleSearch(args map[string]any) (any, error) {
	query := strings.TrimSpace(strArg(args, "query"))
	if query == "" {
		return nil, fmt.Errorf("legal npc: query is required")
	}
	count := intArg(args, "count")
	if count <= 0 {
		count = 10
	}
	if count > maxNPCCount {
		count = maxNPCCount
	}
	base := resolveBaseURL(strArg(args, "base_url"))

	body := npcSearchRequest{
		SearchRange:   1,
		Sxrq:          []string{},
		Gbrq:          []string{},
		Sxx:           []string{},
		GbrqYear:      []string{},
		FlfgCodeID:    []string{},
		ZdjgCodeID:    []string{},
		SearchContent: query,
		XgzlSearch:    false,
		PageNum:       1,
		PageSize:      count,
	}
	body.SearchType = 2
	if boolArg(args, "disable_fuzzy") {
		body.SearchType = 1
	}
	body.OrderByParam.Order = "-1"

	started := time.Now()
	resp, err := doHostHTTP(map[string]any{
		"method": "POST",
		"url":    base + "/law-search/search/list",
		"headers": map[string]string{
			"Content-Type": "application/json;charset=UTF-8",
			"User-Agent":   "agentflow",
		},
		"json": body,
	})
	if err != nil {
		return nil, fmt.Errorf("legal npc: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("legal npc: status %d: %s", resp.Status, truncateStr(resp.Body, 512))
	}

	var sr npcSearchResponse
	if err := json.Unmarshal([]byte(resp.Body), &sr); err != nil {
		return nil, fmt.Errorf("legal npc: decode search response: %w", err)
	}
	if sr.Code != 200 {
		return nil, fmt.Errorf("legal npc: search failed (code %d): %s", sr.Code, sr.Msg)
	}

	hits := make([]npcHit, 0, len(sr.Rows))
	for _, r := range sr.Rows {
		hits = append(hits, npcHit{
			Title:     npcEmRe.ReplaceAllString(r.Title, ""),
			Path:      r.BBBS,
			URL:       base + "/detail2.html?" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(r.BBBS))),
			Court:     r.ZdjgName,
			Date:      r.Gbrq,
			Type:      r.Flxz,
			Effective: r.Sxrq,
			Status:    npcStatus[r.Sxx],
		})
	}

	logMsg(0, fmt.Sprintf("npc.search query=%q hits=%d", query, len(hits)))
	return map[string]any{
		"ok":           true,
		"engine":       "npc",
		"query":        query,
		"total":        sr.Total,
		"count":        len(hits),
		"results":      hits,
		"time_cost_ms": time.Since(started).Milliseconds(),
	}, nil
}

func handleRead(args map[string]any) (any, error) {
	bbbs := strings.TrimSpace(strArg(args, "path"))
	if bbbs == "" {
		return nil, fmt.Errorf("legal npc: need the document id (path from a legal_search hit) to fetch")
	}
	if strings.Contains(bbbs, "/") {
		return nil, fmt.Errorf("legal npc: %q is not a document id (npc ids contain no separators)", bbbs)
	}
	base := resolveBaseURL(strArg(args, "base_url"))

	dlURL := fmt.Sprintf("%s/law-search/download/pc?format=docx&bbbs=%s", base, url.QueryEscape(bbbs))
	metaResp, err := doHostHTTP(map[string]any{
		"method": "GET",
		"url":    dlURL,
		"headers": map[string]string{
			"User-Agent": "agentflow",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("legal npc: %w", err)
	}
	if metaResp.Status != 200 {
		return nil, fmt.Errorf("legal npc: status %d: %s", metaResp.Status, truncateStr(metaResp.Body, 512))
	}

	var dr struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(metaResp.Body), &dr); err != nil {
		return nil, fmt.Errorf("legal npc: decode download response: %w", err)
	}
	if dr.Code != 200 || dr.Data.URL == "" {
		return nil, fmt.Errorf("legal npc: download failed (code %d): %s", dr.Code, dr.Msg)
	}

	binResp, err := doHostHTTP(map[string]any{
		"method": "GET",
		"url":    dr.Data.URL,
		"headers": map[string]string{
			"User-Agent": "agentflow",
		},
		"response_encoding": "base64",
	})
	if err != nil {
		return nil, fmt.Errorf("legal npc: %w", err)
	}
	if binResp.Status != 200 {
		return nil, fmt.Errorf("legal npc: status %d fetching document", binResp.Status)
	}
	docBytes, err := base64.StdEncoding.DecodeString(binResp.BodyBase64)
	if err != nil {
		return nil, fmt.Errorf("legal npc: decode document base64: %w", err)
	}

	text, format, err := Text(docBytes)
	if err != nil {
		return nil, fmt.Errorf("legal npc: extract text: %w", err)
	}

	full := len(text)
	truncated := false
	if start := intArg(args, "start"); start > 0 {
		if start < len(text) {
			text = text[start:]
		} else {
			text = ""
		}
		truncated = true
	}
	if maxChars := intArg(args, "max_chars"); maxChars > 0 && len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	out := map[string]any{
		"ok":       true,
		"engine":   "npc",
		"title":    npcFirstLine(text),
		"neutral":  bbbs,
		"doc":      fmt.Sprintf("%s/law-search/download/pc?format=%s&bbbs=%s", base, format, url.QueryEscape(bbbs)),
		"text":     text,
		"text_len": full,
	}
	if truncated {
		out["truncated"] = true
	}
	logMsg(0, fmt.Sprintf("npc.read bbbs=%s format=%s len=%d", bbbs, format, full))
	return out, nil
}

func resolveBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return defaultNPCURL
	}
	return strings.TrimSuffix(s, "/")
}

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	s, _ := args[key].(string)
	return s
}

func intArg(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func boolArg(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	b, _ := args[key].(bool)
	return b
}

func npcFirstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func main() {}
