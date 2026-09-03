# COMET Integration Registry

This repository is the Alpaca execution adapter used by the COMET Crypto Autopilot project.

## Responsibilities

- Expose bounded Alpaca account, position, and order operations.
- Keep simulation and paper-trading workflows available for autonomous testing.
- Provide a controlled agent-to-Alpaca execution boundary.
- Run automated tests on every push and pull request.

## Safety contract

- Never commit API keys, secrets, access tokens, account data, portfolio state, or order history.
- Credentials belong in protected runtime environment variables only.
- Paper trading is the default integration target.
- Live financial execution requires an explicit, current user authorization.
- No leverage or futures are enabled by this integration.
- A model response alone is never proof that an order filled.
- Unverified execution state must remain UNKNOWN until confirmed by the broker.

## COMET decision gate

A new setup must satisfy all of the following before it can be presented for execution:

1. COMET score of at least 7.5/10.
2. Data confidence of at least 70/100.
3. Three independent signal families.
4. Clear invalidation.
5. Sufficient liquidity.
6. Rational reward-to-risk.
7. Material advantage over cash or the current BTC plan.

## Repository role

GitHub is the auditable source-control and test layer. It does not store brokerage credentials and does not replace account-side confirmation from Alpaca or Revolut X.
