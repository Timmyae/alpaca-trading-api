# alpaca-trading-api

Complete Go API for Alpaca Trading with Agents & GPT integration.

## Features

- REST endpoints for Alpaca account, positions, and order placement
- GPT-powered agent endpoint that translates user instruction into action
- End-to-end `from -> to` flow endpoint for direct automation
- Simulation mode support (no live order execution)
- API token protection (optional)
- Basic per-IP rate limiting
- Input validation, bounded JSON body sizes, and error handling

## Environment Variables

- `PORT` (default: `8080`)
- `APP_API_TOKEN` (optional bearer token for protected routes)
- `ALPACA_BASE_URL` (default: `https://paper-api.alpaca.markets`)
- `ALPACA_API_KEY` (required for Alpaca actions)
- `ALPACA_API_SECRET` (required for Alpaca actions)
- `OPENAI_BASE_URL` (default: `https://api.openai.com/v1`)
- `OPENAI_API_KEY` (required for agent endpoint)
- `OPENAI_MODEL` (default: `gpt-4o-mini`)
- `RATE_LIMIT_PER_MINUTE` (default: `60`)
- `REQUEST_TIMEOUT_SECONDS` (default: `30`)

## Run Locally

```bash
go test ./...
go run .
```

Server starts on `http://localhost:8080` by default.

## Endpoints

### Public

- `GET /health`

### Protected (****** only when `APP_API_TOKEN` is set)

- `GET /v1/alpaca/account`
- `GET /v1/alpaca/positions`
- `POST /v1/alpaca/orders`
- `POST /v1/agent/execute`
- `POST /v1/flow/from-to`

## Example Requests

### Place Alpaca Order

```bash
curl -X POST http://localhost:8080/v1/alpaca/orders \
  -H "Authorization: ******" \
  -H "Content-Type: application/json" \
  -d '{"symbol":"AAPL","qty":"1","side":"buy","type":"market","time_in_force":"day"}'
```

### End-to-End Agent Flow (from -> to)

```bash
curl -X POST http://localhost:8080/v1/flow/from-to \
  -H "Authorization: ******" \
  -H "Content-Type: application/json" \
  -d '{"from":"user","to":"alpaca","instruction":"buy one share of AAPL","simulate":true}'
```

`simulate=true` returns the GPT decision and flow result without executing live orders.
