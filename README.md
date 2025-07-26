# go-rate-limiter

## Self Notes

### TTL Buffer

Adding a TTL Buffer to expiration in redis protects the logic from clock drift, network latency. Also adds a safety margin.

### Gotchas

Go redis client converts float values to int before returning from lua script. So if you want to return a float from lua script, do a `tostring(value)` before returning. Learnt this the hard way.

### IETF RateLimit Headers

IETF defines standard rate-limiting response headers to help clients understand rate limits:

* `RateLimit-Limit`: Max requests allowed in the window
* `RateLimit-Remaining`: Requests left in the current window
* `RateLimit-Reset`: Seconds until the window resets (or a timestamp)

Optional:

* `RateLimit-Policy`: Human-readable rate limit policy (e.g. `100;w=60` → 100 reqs per 60s)
