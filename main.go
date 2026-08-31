package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port               string
	AppToken           string
	AlpacaBaseURL      string
	AlpacaAPIKey       string
	AlpacaAPISecret    string
	OpenAIBaseURL      string
	OpenAIAPIKey       string
	OpenAIModel        string
	RateLimitPerMinute int
	RequestTimeoutSec  int
}

func loadConfig() Config {
	return Config{
		Port:               envOrDefault("PORT", "8080"),
		AppToken:           strings.TrimSpace(os.Getenv("APP_API_TOKEN")),
		AlpacaBaseURL:      envOrDefault("ALPACA_BASE_URL", "https://paper-api.alpaca.markets"),
		AlpacaAPIKey:       strings.TrimSpace(os.Getenv("ALPACA_API_KEY")),
		AlpacaAPISecret:    strings.TrimSpace(os.Getenv("ALPACA_API_SECRET")),
		OpenAIBaseURL:      envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:        envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
		RateLimitPerMinute: envIntOrDefault("RATE_LIMIT_PER_MINUTE", 60),
		RequestTimeoutSec:  envIntOrDefault("REQUEST_TIMEOUT_SECONDS", 30),
	}
}

func envOrDefault(key, def string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	return value
}

func envIntOrDefault(key string, def int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	num, err := strconv.Atoi(value)
	if err != nil || num <= 0 {
		return def
	}
	return num
}

type AlpacaClient struct {
	baseURL string
	apiKey  string
	secret  string
	http    *http.Client
}

func NewAlpacaClient(cfg Config) *AlpacaClient {
	return &AlpacaClient{
		baseURL: strings.TrimSuffix(cfg.AlpacaBaseURL, "/"),
		apiKey:  cfg.AlpacaAPIKey,
		secret:  cfg.AlpacaAPISecret,
		http:    &http.Client{Timeout: time.Duration(cfg.RequestTimeoutSec) * time.Second},
	}
}

func (c *AlpacaClient) requireCredentials() error {
	if c.apiKey == "" || c.secret == "" {
		return errors.New("alpaca credentials are not configured")
	}
	return nil
}

func (c *AlpacaClient) doRequest(ctx context.Context, method, path string, body any) (map[string]any, error) {
	if err := c.requireCredentials(); err != nil {
		return nil, err
	}

	var payload io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encode alpaca request: %w", err)
		}
		payload = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("alpaca error status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed map[string]any
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse alpaca response: %w", err)
	}
	return parsed, nil
}

func (c *AlpacaClient) GetAccount(ctx context.Context) (map[string]any, error) {
	return c.doRequest(ctx, http.MethodGet, "/v2/account", nil)
}

func (c *AlpacaClient) GetPositions(ctx context.Context) ([]map[string]any, error) {
	if err := c.requireCredentials(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v2/positions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("APCA-API-KEY-ID", c.apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("alpaca error status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed []map[string]any
	if len(data) == 0 {
		return []map[string]any{}, nil
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse alpaca positions response: %w", err)
	}
	return parsed, nil
}

type AlpacaOrderRequest struct {
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
}

func validateOrderRequest(in AlpacaOrderRequest) error {
	if strings.TrimSpace(in.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(in.Qty) == "" {
		return errors.New("qty is required")
	}
	side := strings.ToLower(strings.TrimSpace(in.Side))
	if side != "buy" && side != "sell" {
		return errors.New("side must be buy or sell")
	}
	if strings.TrimSpace(in.Type) == "" {
		in.Type = "market"
	}
	if strings.TrimSpace(in.TimeInForce) == "" {
		in.TimeInForce = "day"
	}
	return nil
}

func (c *AlpacaClient) PlaceOrder(ctx context.Context, in AlpacaOrderRequest) (map[string]any, error) {
	if err := validateOrderRequest(in); err != nil {
		return nil, err
	}
	if in.Type == "" {
		in.Type = "market"
	}
	if in.TimeInForce == "" {
		in.TimeInForce = "day"
	}
	return c.doRequest(ctx, http.MethodPost, "/v2/orders", in)
}

type GPTClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewGPTClient(cfg Config) *GPTClient {
	return &GPTClient{
		baseURL: strings.TrimSuffix(cfg.OpenAIBaseURL, "/"),
		apiKey:  cfg.OpenAIAPIKey,
		model:   cfg.OpenAIModel,
		http:    &http.Client{Timeout: time.Duration(cfg.RequestTimeoutSec) * time.Second},
	}
}

type TradeDecision struct {
	Action   string `json:"action"`
	Symbol   string `json:"symbol,omitempty"`
	Qty      string `json:"qty,omitempty"`
	Side     string `json:"side,omitempty"`
	Simulate bool   `json:"simulate"`
	Reason   string `json:"reason,omitempty"`
}

func normalizeAction(action string) string {
	return strings.ToUpper(strings.TrimSpace(action))
}

func (g *GPTClient) AnalyzeInstruction(ctx context.Context, instruction string) (TradeDecision, error) {
	if strings.TrimSpace(g.apiKey) == "" {
		return TradeDecision{}, errors.New("openai api key is not configured")
	}

	system := `You are a strict trading intent parser.
Return ONLY JSON with keys: action, symbol, qty, side, simulate, reason.
Valid action values: BUY, SELL, GET_ACCOUNT, GET_POSITIONS, HOLD.
For BUY/SELL include symbol, qty, side (buy/sell).
If user asks information only, use GET_ACCOUNT or GET_POSITIONS.
If unclear or risky, use HOLD and simulate=true.`

	payload := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": instruction},
		},
		"temperature": 0,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return TradeDecision{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/chat/completions", buf)
	if err != nil {
		return TradeDecision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return TradeDecision{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TradeDecision{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return TradeDecision{}, fmt.Errorf("openai error status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return TradeDecision{}, fmt.Errorf("parse openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return TradeDecision{}, errors.New("openai returned no choices")
	}

	jsonText, err := extractJSONObject(parsed.Choices[0].Message.Content)
	if err != nil {
		return TradeDecision{}, err
	}

	var decision TradeDecision
	if err := json.Unmarshal([]byte(jsonText), &decision); err != nil {
		return TradeDecision{}, fmt.Errorf("invalid decision json: %w", err)
	}
	decision.Action = normalizeAction(decision.Action)
	decision.Side = strings.ToLower(strings.TrimSpace(decision.Side))
	decision.Symbol = strings.ToUpper(strings.TrimSpace(decision.Symbol))
	decision.Qty = strings.TrimSpace(decision.Qty)
	if decision.Action == "BUY" || decision.Action == "SELL" {
		if decision.Symbol == "" || decision.Qty == "" {
			return TradeDecision{}, errors.New("decision for BUY/SELL must include symbol and qty")
		}
		if decision.Side == "" {
			decision.Side = strings.ToLower(decision.Action)
		}
	}
	if decision.Action == "" {
		decision.Action = "HOLD"
		decision.Simulate = true
	}
	return decision, nil
}

func extractJSONObject(content string) (string, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		return content, nil
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end <= start {
		return "", errors.New("model response did not include valid JSON object")
	}
	return content[start : end+1], nil
}

type AgentRequest struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Instruction string `json:"instruction"`
	Simulate    bool   `json:"simulate"`
}

func validateAgentRequest(in AgentRequest) error {
	if strings.TrimSpace(in.From) == "" {
		return errors.New("from is required")
	}
	if strings.TrimSpace(in.To) == "" {
		return errors.New("to is required")
	}
	if strings.TrimSpace(in.Instruction) == "" {
		return errors.New("instruction is required")
	}
	return nil
}

type AgentResult struct {
	Flow          string         `json:"flow"`
	Decision      TradeDecision  `json:"decision"`
	Executed      bool           `json:"executed"`
	Simulation    bool           `json:"simulation"`
	ExecutionData map[string]any `json:"execution_data,omitempty"`
}

type AgentService struct {
	alpaca *AlpacaClient
	gpt    *GPTClient
}

func NewAgentService(alpaca *AlpacaClient, gpt *GPTClient) *AgentService {
	return &AgentService{alpaca: alpaca, gpt: gpt}
}

func (a *AgentService) Execute(ctx context.Context, req AgentRequest) (AgentResult, error) {
	if err := validateAgentRequest(req); err != nil {
		return AgentResult{}, err
	}

	decision, err := a.gpt.AnalyzeInstruction(ctx, req.Instruction)
	if err != nil {
		return AgentResult{}, err
	}

	result := AgentResult{
		Flow:       fmt.Sprintf("%s->%s", strings.TrimSpace(req.From), strings.TrimSpace(req.To)),
		Decision:   decision,
		Simulation: req.Simulate || decision.Simulate || decision.Action == "HOLD",
	}

	if result.Simulation {
		return result, nil
	}

	switch decision.Action {
	case "BUY", "SELL":
		order, err := a.alpaca.PlaceOrder(ctx, AlpacaOrderRequest{
			Symbol:      decision.Symbol,
			Qty:         decision.Qty,
			Side:        decision.Side,
			Type:        "market",
			TimeInForce: "day",
		})
		if err != nil {
			return AgentResult{}, err
		}
		result.Executed = true
		result.ExecutionData = order
	case "GET_ACCOUNT":
		account, err := a.alpaca.GetAccount(ctx)
		if err != nil {
			return AgentResult{}, err
		}
		result.Executed = true
		result.ExecutionData = account
	case "GET_POSITIONS":
		positions, err := a.alpaca.GetPositions(ctx)
		if err != nil {
			return AgentResult{}, err
		}
		result.Executed = true
		result.ExecutionData = map[string]any{"positions": positions}
	default:
		result.Simulation = true
	}
	return result, nil
}

type rateState struct {
	Count       int
	WindowStart time.Time
}

type RateLimiter struct {
	limit int
	mu    sync.Mutex
	data  map[string]rateState
}

func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{limit: limit, data: map[string]rateState{}}
}

func (r *RateLimiter) Allow(key string) bool {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()

	state := r.data[key]
	if state.WindowStart.IsZero() || now.Sub(state.WindowStart) >= time.Minute {
		r.data[key] = rateState{Count: 1, WindowStart: now}
		return true
	}
	if state.Count >= r.limit {
		return false
	}
	state.Count++
	r.data[key] = state
	return true
}

type Server struct {
	cfg         Config
	alpaca      *AlpacaClient
	agent       *AgentService
	rateLimiter *RateLimiter
}

func NewServer(cfg Config) *Server {
	alpaca := NewAlpacaClient(cfg)
	gpt := NewGPTClient(cfg)
	return &Server{
		cfg:         cfg,
		alpaca:      alpaca,
		agent:       NewAgentService(alpaca, gpt),
		rateLimiter: NewRateLimiter(cfg.RateLimitPerMinute),
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/alpaca/account", s.withProtected(s.handleGetAccount))
	mux.HandleFunc("GET /v1/alpaca/positions", s.withProtected(s.handleGetPositions))
	mux.HandleFunc("POST /v1/alpaca/orders", s.withProtected(s.handlePlaceOrder))
	mux.HandleFunc("POST /v1/agent/execute", s.withProtected(s.handleAgentExecute))
	mux.HandleFunc("POST /v1/flow/from-to", s.withProtected(s.handleAgentExecute))
	return s.withRateLimit(mux)
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !s.rateLimiter.Allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withProtected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AppToken != "" {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			expected := "Bearer " + s.cfg.AppToken
			if header != expected {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	account, err := s.alpaca.GetAccount(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (s *Server) handleGetPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := s.alpaca.GetPositions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"positions": positions})
}

func (s *Server) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	var req AlpacaOrderRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	order, err := s.alpaca.PlaceOrder(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleAgentExecute(w http.ResponseWriter, r *http.Request) {
	var req AgentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := s.agent.Execute(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func main() {
	cfg := loadConfig()
	server := NewServer(cfg)
	addr := ":" + cfg.Port
	log.Printf("starting server on %s", addr)
	if err := http.ListenAndServe(addr, server.routes()); err != nil {
		log.Fatal(err)
	}
}
