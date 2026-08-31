package main

import "testing"

func TestValidateOrderRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   AlpacaOrderRequest
		wantErr bool
	}{
		{name: "valid buy", input: AlpacaOrderRequest{Symbol: "AAPL", Qty: "1", Side: "buy"}},
		{name: "invalid side", input: AlpacaOrderRequest{Symbol: "AAPL", Qty: "1", Side: "hold"}, wantErr: true},
		{name: "missing symbol", input: AlpacaOrderRequest{Qty: "1", Side: "buy"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrderRequest(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain json", input: `{"action":"HOLD"}`, want: `{"action":"HOLD"}`},
		{name: "json in markdown", input: "```json\n{\"action\":\"BUY\"}\n```", want: `{"action":"BUY"}`},
		{name: "no json", input: "hello", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONObject(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateAgentRequest(t *testing.T) {
	if err := validateAgentRequest(AgentRequest{From: "user", To: "alpaca", Instruction: "buy one share"}); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}
	if err := validateAgentRequest(AgentRequest{To: "alpaca", Instruction: "buy"}); err == nil {
		t.Fatalf("expected error for missing from")
	}
}
